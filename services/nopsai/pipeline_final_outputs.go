package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/config"
	"nopsai/pkg/llmclient"
	"nopsai/pkg/models"
	runquery "nopsai/services/nopsai/internal/runs"
)

const (
	pipelineFinalOutputFeature      = "pipeline_final_output"
	finalOutputStatusPending        = "pending"
	finalOutputStatusRunning        = "generating"
	finalOutputStatusSuccess        = "success"
	finalOutputStatusFailure        = "failure"
	finalOutputStatusCancelled      = "cancelled"
	pipelineFinalOutputHistoryLimit = 5
	pipelineFinalOutputLogDrainWait = 750 * time.Millisecond
)

var (
	pipelineFinalOutputImageRefPattern      = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9._/-]*:[A-Za-z0-9][A-Za-z0-9._-]*\b`)
	pipelineFinalOutputDurationTokenPattern = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*(?:ms|s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\b`)
	pipelineFinalOutputEvidenceKeyPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,64}_evidence$`)
)

type pipelineFinalOutputRecord struct {
	models.PipelineRunFinalOutput
	ItemIndex int
	Prompt    string
	Dashboard models.DashboardOutputTarget
}

type pipelineFinalOutputRunContext struct {
	Text        string
	Scope       string
	LogEvidence pipelineFinalOutputLogEvidence
}

type pipelineFinalOutputLogEvidence struct {
	Lines      []string
	Structured []string
	Images     []pipelineFinalOutputImageEvidence
}

type pipelineFinalOutputStructuredImageEvidence struct {
	Name               string
	Tag                string
	Environment        string
	DurationSeconds    float64
	HasDuration        bool
	Vulnerable         bool
	HasVulnerability   bool
	MissingRuntime     bool
	HasMissingRuntime  bool
	ProductionReady    bool
	HasProductionReady bool
}

var (
	errPipelineFinalOutputNotCancellable = errors.New("final output is already complete")
	errPipelineFinalOutputNotRetryable   = errors.New("final output is not failed")
)

func (a *App) preparePipelineFinalOutputRecords(ctx context.Context, runID string) error {
	record, err := runquery.LoadRunRecord(ctx, a.db, runID)
	if err != nil {
		return err
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(record.PipelineDefinitionYAML), &pipeline); err != nil {
		return fmt.Errorf("parse pipeline output definition: %w", err)
	}
	if len(pipeline.Output.Items) == 0 {
		return nil
	}
	runStatus := runquery.NormalizeRunDetailStatus(record.Run.Status)

	defaultProfile, _, err := a.pipelineFinalOutputProfiles(ctx)
	if err != nil {
		return err
	}
	for idx, item := range pipeline.Output.Items {
		if !pipelineFinalOutputMatchesRunStatus(item.When, runStatus) {
			continue
		}
		name := strings.TrimSpace(item.Name)
		outputType := normalizePipelineFinalOutputType(item.Type)
		prompt := strings.TrimSpace(item.Prompt)
		profileName := resolvePipelineFinalOutputProfileName(defaultProfile, pipeline, item)
		if name == "" || outputType == "" || prompt == "" {
			continue
		}
		_, err := a.db.Exec(ctx, `
			INSERT INTO pipeline_run_outputs (
				run_id, item_index, name, type, prompt, llm_profile, dashboard_target, status, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, 'pending', NOW())
			ON CONFLICT (run_id, item_index) DO UPDATE SET
				name = EXCLUDED.name,
				type = EXCLUDED.type,
				prompt = EXCLUDED.prompt,
				llm_profile = EXCLUDED.llm_profile,
				dashboard_target = EXCLUDED.dashboard_target,
				status = CASE
					WHEN pipeline_run_outputs.status IN ('success', 'cancelled') THEN pipeline_run_outputs.status
					ELSE 'pending'
				END,
				error = CASE
					WHEN pipeline_run_outputs.status IN ('success', 'cancelled') THEN pipeline_run_outputs.error
					ELSE ''
				END,
				generation_attempts = CASE
					WHEN pipeline_run_outputs.status IN ('success', 'cancelled') THEN pipeline_run_outputs.generation_attempts
					ELSE 0
				END,
				contract_violations = CASE
					WHEN pipeline_run_outputs.status IN ('success', 'cancelled') THEN pipeline_run_outputs.contract_violations
					ELSE 0
				END,
				generation_started_at = CASE
					WHEN pipeline_run_outputs.status IN ('success', 'cancelled') THEN pipeline_run_outputs.generation_started_at
					ELSE NULL
				END,
				updated_at = NOW()
		`, runID, idx, name, outputType, prompt, profileName, mustMarshalDashboardTarget(item.Dashboard))
		if err != nil {
			return fmt.Errorf("prepare final output %q: %w", name, err)
		}
	}
	return nil
}

func (a *App) generatePipelineFinalOutputs(ctx context.Context, runID string) {
	if a == nil || a.db == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.preparePipelineFinalOutputRecords(ctx, runID); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("Failed to prepare pipeline final outputs")
		return
	}
	outputs, err := a.loadPipelineFinalOutputsForGeneration(ctx, runID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("Failed to load pipeline final outputs")
		return
	}
	if len(outputs) == 0 {
		return
	}
	if err := a.waitForPipelineFinalOutputLogDrain(ctx); err != nil {
		log.Debug().Err(err).Str("run_id", runID).Msg("Pipeline final output log drain wait skipped")
	}
	runContext, err := a.buildPipelineFinalOutputRunContext(ctx, runID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("Failed to build pipeline final output context")
		for _, output := range outputs {
			_ = a.markPipelineFinalOutputFailure(ctx, output.ID, err)
			a.markDashboardRefreshOutputFailureIfDashboard(ctx, runID, output, err)
		}
		return
	}

	for _, output := range outputs {
		if err := a.generatePipelineFinalOutput(ctx, runID, runContext, output); err != nil {
			log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to generate pipeline final output")
		}
	}
}

func (a *App) waitForPipelineFinalOutputLogDrain(ctx context.Context) error {
	if a == nil || a.db == nil || pipelineFinalOutputLogDrainWait <= 0 {
		return nil
	}
	timer := time.NewTimer(pipelineFinalOutputLogDrainWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *App) loadPipelineFinalOutputsForGeneration(ctx context.Context, runID string) ([]pipelineFinalOutputRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, item_index, name, type, prompt, llm_profile, status, content, error,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, generation_started_at, updated_at, dashboard_target::text
		FROM pipeline_run_outputs
		WHERE run_id = $1 AND status IN ('pending', 'generating')
		ORDER BY item_index ASC, created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outputs := []pipelineFinalOutputRecord{}
	for rows.Next() {
		var output pipelineFinalOutputRecord
		var dashboardRaw string
		var generationStartedAt sql.NullTime
		if err := rows.Scan(
			&output.ID,
			&output.ItemIndex,
			&output.Name,
			&output.Type,
			&output.Prompt,
			&output.LLMProfile,
			&output.Status,
			&output.Content,
			&output.Error,
			&output.GenerationAttempts,
			&output.ContractViolations,
			&output.RenderAttempts,
			&output.RenderFailures,
			&output.CreatedAt,
			&generationStartedAt,
			&output.UpdatedAt,
			&dashboardRaw,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(dashboardRaw), &output.Dashboard)
		output.DashboardTarget = pipelineFinalOutputDashboardTargetPtr(output.Dashboard)
		output.GenerationStartedAt = nullTimePtr(generationStartedAt)
		outputs = append(outputs, output)
	}
	return outputs, rows.Err()
}

func (a *App) generatePipelineFinalOutput(ctx context.Context, runID string, runContext pipelineFinalOutputRunContext, output pipelineFinalOutputRecord) error {
	claimed, err := a.claimPipelineFinalOutputForGeneration(ctx, output.ID)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	generationCtx, releaseGeneration := a.registerPipelineFinalOutputGeneration(ctx, output.ID)
	defer releaseGeneration()

	if normalizePipelineFinalOutputType(output.Type) == "dashboard" {
		if err := a.markDashboardRefreshOutputGenerating(ctx, runID, output); err != nil {
			log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to mark dashboard refresh output generating")
		}
	}

	client, err := a.pipelineFinalOutputLLMClient(ctx, output.LLMProfile, runContext.Scope)
	if err != nil {
		_ = a.markPipelineFinalOutputFailure(ctx, output.ID, err)
		a.markDashboardRefreshOutputFailureIfDashboard(ctx, runID, output, err)
		return err
	}
	prompt := buildPipelineFinalOutputPrompt(runContext.Text, output)
	result, err := generateValidatedPipelineFinalOutput(generationCtx, client, output.Type, prompt, pipelineFinalOutputRecordContentValidator(output))
	a.recordPipelineFinalOutputAttemptUsage(ctx, runID, output, result)
	if err != nil {
		cancelled, cancelErr := a.pipelineFinalOutputCancelled(ctx, output.ID)
		if cancelErr != nil {
			return cancelErr
		}
		if cancelled {
			return nil
		}
		_ = a.markPipelineFinalOutputFailureWithResult(ctx, output.ID, err, result)
		a.markDashboardRefreshOutputFailureIfDashboard(ctx, runID, output, err)
		return err
	}
	cancelled, err := a.pipelineFinalOutputCancelled(ctx, output.ID)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}
	if normalizePipelineFinalOutputType(output.Type) == "dashboard" {
		content, err := groundPipelineFinalDashboardOutputContent(result.Content, output, runContext.LogEvidence)
		if err != nil {
			_ = a.markPipelineFinalOutputFailureWithResult(ctx, output.ID, err, result)
			a.markDashboardRefreshOutputFailureIfDashboard(ctx, runID, output, err)
			return err
		}
		result.Content = content
		cancelled, err := a.pipelineFinalOutputCancelled(ctx, output.ID)
		if err != nil {
			return err
		}
		if cancelled {
			return nil
		}
		if err := a.publishDashboardFinalOutput(ctx, runID, output, result.Content); err != nil {
			_ = a.markPipelineFinalOutputFailureWithResult(ctx, output.ID, err, result)
			a.markDashboardRefreshOutputFailureIfDashboard(ctx, runID, output, err)
			return err
		}
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'success', content = $2, error = '',
		    generation_attempts = $3, contract_violations = $4, updated_at = NOW()
		WHERE id = $1 AND status <> 'cancelled'
	`, output.ID, result.Content, len(result.Attempts), result.ContractViolations)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return nil
}

func (a *App) claimPipelineFinalOutputForGeneration(ctx context.Context, outputID string) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'generating', error = '', generation_started_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'generating')
	`, outputID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (a *App) registerPipelineFinalOutputGeneration(ctx context.Context, outputID string) (context.Context, func()) {
	generationCtx, cancel := context.WithCancel(ctx)
	if a != nil && strings.TrimSpace(outputID) != "" {
		a.finalOutputCancellers.Store(outputID, cancel)
	}
	return generationCtx, func() {
		cancel()
		if a != nil && strings.TrimSpace(outputID) != "" {
			a.finalOutputCancellers.Delete(outputID)
		}
	}
}

func (a *App) pipelineFinalOutputCancelled(ctx context.Context, outputID string) (bool, error) {
	var status string
	err := a.db.QueryRow(ctx, `
		SELECT status
		FROM pipeline_run_outputs
		WHERE id = $1
	`, outputID).Scan(&status)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(status), finalOutputStatusCancelled), nil
}

func (a *App) markDashboardRefreshOutputFailureIfDashboard(ctx context.Context, runID string, output pipelineFinalOutputRecord, cause error) {
	if normalizePipelineFinalOutputType(output.Type) != "dashboard" {
		return
	}
	if err := a.markDashboardRefreshOutputFailed(ctx, runID, output, cause); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to mark dashboard refresh output failed")
	}
}

func (a *App) cancelPipelineFinalOutput(ctx context.Context, runID, outputID string) (pipelineFinalOutputRecord, error) {
	output, err := a.updatePipelineFinalOutputCancelled(ctx, runID, outputID)
	if err == nil {
		a.cancelActivePipelineFinalOutputGeneration(output.ID)
		a.markDashboardRefreshOutputCancelledIfDashboard(ctx, runID, output)
		return output, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pipelineFinalOutputRecord{}, err
	}
	output, err = a.loadPipelineFinalOutputRecord(ctx, runID, outputID)
	if err != nil {
		return pipelineFinalOutputRecord{}, err
	}
	if strings.EqualFold(strings.TrimSpace(output.Status), finalOutputStatusCancelled) {
		a.cancelActivePipelineFinalOutputGeneration(output.ID)
		return output, nil
	}
	return pipelineFinalOutputRecord{}, errPipelineFinalOutputNotCancellable
}

func (a *App) retryPipelineFinalOutput(ctx context.Context, runID, outputID string) (pipelineFinalOutputRecord, error) {
	output, err := a.resetPipelineFinalOutputForRetry(ctx, runID, outputID)
	if err == nil {
		go a.generatePipelineFinalOutputByID(context.Background(), runID, output.ID)
		return output, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pipelineFinalOutputRecord{}, err
	}
	_, err = a.loadPipelineFinalOutputRecord(ctx, runID, outputID)
	if err != nil {
		return pipelineFinalOutputRecord{}, err
	}
	return pipelineFinalOutputRecord{}, errPipelineFinalOutputNotRetryable
}

func (a *App) generatePipelineFinalOutputByID(ctx context.Context, runID, outputID string) {
	if a == nil || a.db == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.waitForPipelineFinalOutputLogDrain(ctx); err != nil {
		log.Debug().Err(err).Str("run_id", runID).Str("output_id", outputID).Msg("Pipeline final output retry log drain wait skipped")
	}
	output, err := a.loadPipelineFinalOutputRecord(ctx, runID, outputID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("output_id", outputID).Msg("Failed to load pipeline final output for retry")
		return
	}
	status := strings.TrimSpace(output.Status)
	if status != finalOutputStatusPending && status != finalOutputStatusRunning {
		return
	}
	runContext, err := a.buildPipelineFinalOutputRunContext(ctx, runID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to build pipeline final output retry context")
		_ = a.markPipelineFinalOutputFailure(ctx, output.ID, err)
		a.markDashboardRefreshOutputFailureIfDashboard(ctx, runID, output, err)
		return
	}
	if err := a.generatePipelineFinalOutput(ctx, runID, runContext, output); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to retry pipeline final output")
	}
}

func (a *App) cancelActivePipelineFinalOutputGeneration(outputID string) {
	if a == nil || strings.TrimSpace(outputID) == "" {
		return
	}
	value, ok := a.finalOutputCancellers.Load(outputID)
	if !ok {
		return
	}
	if cancel, ok := value.(context.CancelFunc); ok {
		cancel()
	}
}

func (a *App) updatePipelineFinalOutputCancelled(ctx context.Context, runID, outputID string) (pipelineFinalOutputRecord, error) {
	return scanPipelineFinalOutputRecord(a.db.QueryRow(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'cancelled',
			error = 'cancelled by user',
			updated_at = NOW()
		WHERE run_id::text = $1
		  AND id::text = $2
		  AND status IN ('pending', 'generating')
		RETURNING id::text, item_index, name, type, prompt, llm_profile, status, content, error,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, generation_started_at, updated_at, dashboard_target::text
	`, runID, outputID))
}

func (a *App) resetPipelineFinalOutputForRetry(ctx context.Context, runID, outputID string) (pipelineFinalOutputRecord, error) {
	return scanPipelineFinalOutputRecord(a.db.QueryRow(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'pending',
			content = '',
			error = '',
			generation_attempts = 0,
			contract_violations = 0,
			render_attempts = 0,
			render_failures = 0,
			generation_started_at = NULL,
			updated_at = NOW()
		WHERE run_id::text = $1
		  AND id::text = $2
		  AND status = 'failure'
		RETURNING id::text, item_index, name, type, prompt, llm_profile, status, content, error,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, generation_started_at, updated_at, dashboard_target::text
	`, runID, outputID))
}

func (a *App) loadPipelineFinalOutputRecord(ctx context.Context, runID, outputID string) (pipelineFinalOutputRecord, error) {
	return scanPipelineFinalOutputRecord(a.db.QueryRow(ctx, `
		SELECT id::text, item_index, name, type, prompt, llm_profile, status, content, error,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, generation_started_at, updated_at, dashboard_target::text
		FROM pipeline_run_outputs
		WHERE run_id::text = $1 AND id::text = $2
	`, runID, outputID))
}

func scanPipelineFinalOutputRecord(scanner interface{ Scan(dest ...any) error }) (pipelineFinalOutputRecord, error) {
	var output pipelineFinalOutputRecord
	var dashboardRaw string
	var generationStartedAt sql.NullTime
	if err := scanner.Scan(
		&output.ID,
		&output.ItemIndex,
		&output.Name,
		&output.Type,
		&output.Prompt,
		&output.LLMProfile,
		&output.Status,
		&output.Content,
		&output.Error,
		&output.GenerationAttempts,
		&output.ContractViolations,
		&output.RenderAttempts,
		&output.RenderFailures,
		&output.CreatedAt,
		&generationStartedAt,
		&output.UpdatedAt,
		&dashboardRaw,
	); err != nil {
		return output, err
	}
	output.GenerationStartedAt = nullTimePtr(generationStartedAt)
	output.GenerationDuration, output.GenerationSeconds = runquery.FinalOutputGenerationTiming(output.GenerationStartedAt, output.UpdatedAt)
	_ = json.Unmarshal([]byte(dashboardRaw), &output.Dashboard)
	output.DashboardTarget = pipelineFinalOutputDashboardTargetPtr(output.Dashboard)
	return output, nil
}

func pipelineFinalOutputDashboardTargetPtr(target models.DashboardOutputTarget) *models.DashboardOutputTarget {
	if strings.TrimSpace(target.Ref) == "" &&
		strings.TrimSpace(target.Section) == "" &&
		strings.TrimSpace(target.EntryKey) == "" &&
		strings.TrimSpace(target.Mode) == "" &&
		strings.TrimSpace(target.Preset) == "" &&
		strings.TrimSpace(target.TTL) == "" {
		return nil
	}
	return &target
}

func (a *App) markDashboardRefreshOutputCancelledIfDashboard(ctx context.Context, runID string, output pipelineFinalOutputRecord) {
	if normalizePipelineFinalOutputType(output.Type) != "dashboard" {
		return
	}
	if err := a.markDashboardRefreshOutputCancelled(ctx, runID, output); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to mark dashboard refresh output cancelled")
	}
}

func pipelineFinalOutputRecordContentValidator(output pipelineFinalOutputRecord) pipelineFinalOutputContentValidator {
	if normalizePipelineFinalOutputType(output.Type) != "dashboard" {
		return nil
	}
	if normalizeDashboardPublishMode(output.Dashboard.Mode) != dashboardPublishModeSeries {
		return nil
	}
	return func(content string) error {
		spec, err := parseDashboardSpec(content)
		if err != nil {
			return newPipelineFinalOutputContractError("invalid_dashboard_spec", err.Error())
		}
		if err := validateDashboardSeriesPublicationSpec(spec); err != nil {
			return newPipelineFinalOutputContractError("invalid_dashboard_spec", err.Error())
		}
		return nil
	}
}

func groundPipelineFinalDashboardOutputContent(content string, output pipelineFinalOutputRecord, evidence pipelineFinalOutputLogEvidence) (string, error) {
	if normalizePipelineFinalOutputType(output.Type) != "dashboard" {
		return content, nil
	}
	counts := pipelineFinalOutputStructuredDashboardCounts(evidence)
	if len(counts) == 0 {
		return content, nil
	}
	var spec models.DashboardSpec
	if err := json.Unmarshal([]byte(content), &spec); err != nil {
		return "", fmt.Errorf("parse dashboard output for grounding: %w", err)
	}
	changed := false
	if pipelineFinalOutputRequestsOperationsOverview(output) {
		if overview, ok := dashboardSpecFromPipelineFinalOutputOperationsOverview(evidence, counts); ok {
			spec = overview
			changed = true
		}
	}
	changed = groundDashboardCircularChartsFromEvidence(&spec, counts) || changed
	if pipelineFinalOutputRequestsBuildDurationMetrics(output) {
		changed = groundDashboardBuildDurationMetricsFromEvidence(&spec, pipelineFinalOutputStructuredDashboardImages(evidence)) || changed
	}
	if !changed {
		return content, nil
	}
	payload, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("encode grounded dashboard output: %w", err)
	}
	return string(payload), nil
}

func pipelineFinalOutputStructuredDashboardCounts(evidence pipelineFinalOutputLogEvidence) map[string]float64 {
	counts := map[string]float64{}
	for _, line := range evidence.Structured {
		_, payload, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &root); err != nil {
			continue
		}
		for _, key := range []string{"images_built", "git_changelog_updated"} {
			if value, ok := dashboardEvidenceNumber(root[key]); ok {
				addDashboardEvidenceCount(counts, key, value)
			}
		}
		if rawSummary, ok := root["readiness_summary"]; ok {
			var summary map[string]json.RawMessage
			if err := json.Unmarshal(rawSummary, &summary); err == nil {
				for key, raw := range summary {
					if value, ok := dashboardEvidenceNumber(raw); ok {
						addDashboardEvidenceCount(counts, key, value)
					}
				}
			}
		}
		if rawImages, ok := root["images"]; ok {
			addDashboardImageDerivedEvidenceCounts(counts, rawImages)
		}
	}
	return counts
}

func pipelineFinalOutputStructuredDashboardImages(evidence pipelineFinalOutputLogEvidence) []pipelineFinalOutputStructuredImageEvidence {
	images := []pipelineFinalOutputStructuredImageEvidence{}
	for _, line := range evidence.Structured {
		_, payload, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &root); err != nil {
			continue
		}
		rawImages, ok := root["images"]
		if !ok {
			continue
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(rawImages, &rows); err != nil {
			continue
		}
		for _, row := range rows {
			image := pipelineFinalOutputStructuredImageEvidence{
				Name:        dashboardEvidenceString(row["name"]),
				Tag:         dashboardEvidenceString(row["tag"]),
				Environment: dashboardEvidenceString(row["environment"]),
			}
			if value, ok := dashboardEvidenceNumber(row["build_duration_seconds"]); ok {
				image.DurationSeconds = value
				image.HasDuration = true
			} else if value, ok := pipelineFinalOutputDurationSeconds(dashboardEvidenceString(row["build_duration"])); ok {
				image.DurationSeconds = value
				image.HasDuration = true
			}
			if value, ok := dashboardEvidenceBool(row["has_vulnerabilities"]); ok {
				image.Vulnerable = value
				image.HasVulnerability = true
			}
			if value, ok := dashboardEvidenceBool(row["missing_environment"]); ok {
				image.MissingRuntime = value
				image.HasMissingRuntime = true
			}
			if value, ok := dashboardEvidenceBool(row["production_ready"]); ok {
				image.ProductionReady = value
				image.HasProductionReady = true
			}
			if image.Name != "" {
				images = append(images, image)
			}
		}
	}
	return images
}

func addDashboardImageDerivedEvidenceCounts(counts map[string]float64, raw json.RawMessage) {
	var images []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &images); err != nil || len(images) == 0 {
		return
	}
	var productionReady, missingConfiguration, vulnerable float64
	for _, image := range images {
		if value, ok := dashboardEvidenceBool(image["production_ready"]); ok && value {
			productionReady++
		}
		if value, ok := dashboardEvidenceBool(image["missing_environment"]); ok && value {
			missingConfiguration++
		}
		if value, ok := dashboardEvidenceBool(image["has_vulnerabilities"]); ok && value {
			vulnerable++
		}
	}
	total := float64(len(images))
	addDashboardEvidenceCount(counts, "images_built", total)
	addDashboardEvidenceCount(counts, "production_ready", productionReady)
	addDashboardEvidenceCount(counts, "blocked_from_production", total-productionReady)
	addDashboardEvidenceCount(counts, "missing_runtime_configuration", missingConfiguration)
	addDashboardEvidenceCount(counts, "runtime_configuration_present", total-missingConfiguration)
	addDashboardEvidenceCount(counts, "vulnerable_images", vulnerable)
}

func dashboardEvidenceString(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return strings.TrimSpace(strconv.FormatFloat(number, 'f', -1, 64))
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		return strconv.FormatBool(boolean)
	}
	return ""
}

func dashboardEvidenceNumber(raw json.RawMessage) (float64, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		if boolean {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func dashboardEvidenceBool(raw json.RawMessage) (bool, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}

func addDashboardEvidenceCount(counts map[string]float64, key string, value float64) {
	normalized := dashboardEvidenceCountKey(key)
	if normalized == "" {
		return
	}
	counts[normalized] = value
	for _, alias := range dashboardEvidenceCountAliases(normalized) {
		counts[alias] = value
	}
}

func dashboardEvidenceCountAliases(key string) []string {
	switch key {
	case "missing_runtime_configuration":
		return []string{"missing_configuration", "missing_environment", "missing_env_config"}
	case "runtime_configuration_present":
		return []string{"configuration_present", "environment_present", "env_config_present"}
	case "blocked_from_production":
		return []string{"blocked", "production_blocked", "blocked_images"}
	case "production_ready":
		return []string{"ready", "production_ready_images"}
	case "vulnerable_images":
		return []string{"has_vulnerabilities", "vulnerabilities", "vulnerable"}
	default:
		return nil
	}
}

func dashboardEvidenceCountKey(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}
	return strings.Trim(normalized, "_")
}

func groundDashboardCircularChartsFromEvidence(spec *models.DashboardSpec, counts map[string]float64) bool {
	if spec == nil || len(counts) == 0 {
		return false
	}
	changed := false
	for blockIndex := range spec.Blocks {
		block := &spec.Blocks[blockIndex]
		if block.Chart == nil {
			continue
		}
		chartType := strings.ToLower(strings.TrimSpace(block.Chart.Type))
		if chartType != "pie" && chartType != "donut" {
			continue
		}
		if dashboardChartHasPoints(block.Chart) {
			continue
		}
		points := dashboardCircularPointsFromEvidenceSeries(block.Chart.Series, counts)
		if len(points) == 0 {
			continue
		}
		block.Chart.Series = []models.DashboardChartSeries{
			{
				Key:    dashboardGroundedChartSeriesKey(block, chartType),
				Label:  dashboardGroundedChartSeriesLabel(block),
				Points: points,
			},
		}
		changed = true
	}
	return changed
}

func dashboardChartHasPoints(chart *models.DashboardChart) bool {
	if chart == nil {
		return false
	}
	for _, series := range chart.Series {
		if len(series.Points) > 0 {
			return true
		}
	}
	return false
}

func dashboardCircularPointsFromEvidenceSeries(series []models.DashboardChartSeries, counts map[string]float64) []models.DashboardSeriesPoint {
	points := make([]models.DashboardSeriesPoint, 0, len(series))
	for _, item := range series {
		value, ok := dashboardEvidenceCountForSeries(item, counts)
		if !ok {
			continue
		}
		pointValue := value
		points = append(points, models.DashboardSeriesPoint{
			Label: dashboardEvidenceSeriesPointLabel(item),
			Value: &pointValue,
		})
	}
	return points
}

func dashboardEvidenceCountForSeries(series models.DashboardChartSeries, counts map[string]float64) (float64, bool) {
	for _, key := range []string{series.Key, series.Label} {
		normalized := dashboardEvidenceCountKey(key)
		if normalized == "" {
			continue
		}
		if value, ok := counts[normalized]; ok {
			return value, true
		}
		for _, alias := range dashboardEvidenceCountAliases(normalized) {
			if value, ok := counts[alias]; ok {
				return value, true
			}
		}
	}
	return 0, false
}

func dashboardEvidenceSeriesPointLabel(series models.DashboardChartSeries) string {
	if label := strings.TrimSpace(series.Label); label != "" {
		return dashboardEvidenceDisplayLabel(label)
	}
	if key := strings.TrimSpace(series.Key); key != "" {
		return dashboardEvidenceDisplayLabel(key)
	}
	return "Value"
}

func dashboardEvidenceDisplayLabel(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return "Value"
	}
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func pipelineFinalOutputRequestsOperationsOverview(output pipelineFinalOutputRecord) bool {
	text := strings.ToLower(strings.Join([]string{
		output.Name,
		output.Prompt,
		output.Dashboard.EntryKey,
		output.Dashboard.Preset,
	}, " "))
	return strings.Contains(text, "operations digest") ||
		strings.Contains(text, "operational overview") ||
		strings.Contains(text, "operations overview") ||
		strings.Contains(text, "mixed dashboard digest")
}

func dashboardSpecFromPipelineFinalOutputOperationsOverview(evidence pipelineFinalOutputLogEvidence, counts map[string]float64) (models.DashboardSpec, bool) {
	images := pipelineFinalOutputStructuredDashboardImages(evidence)
	metrics, ok := dashboardBuildDurationEvidenceMetrics(images)
	if !ok {
		return models.DashboardSpec{}, false
	}
	total := dashboardCountValue(counts, "images_built", float64(len(images)))
	productionReady := dashboardCountValue(counts, "production_ready", dashboardImageBoolCount(images, func(image pipelineFinalOutputStructuredImageEvidence) (bool, bool) {
		return image.ProductionReady, image.HasProductionReady
	}))
	blocked := dashboardCountValue(counts, "blocked_from_production", total-productionReady)
	missingRuntime := dashboardCountValue(counts, "missing_runtime_configuration", dashboardImageBoolCount(images, func(image pipelineFinalOutputStructuredImageEvidence) (bool, bool) {
		return image.MissingRuntime, image.HasMissingRuntime
	}))
	configPresent := dashboardCountValue(counts, "runtime_configuration_present", total-missingRuntime)
	vulnerable := dashboardCountValue(counts, "vulnerable_images", dashboardImageBoolCount(images, func(image pipelineFinalOutputStructuredImageEvidence) (bool, bool) {
		return image.Vulnerable, image.HasVulnerability
	}))
	changelogUpdated := dashboardCountValue(counts, "git_changelog_updated", 0) > 0
	riskText := pipelineFinalOutputStructuredDashboardString(evidence, "operational_risk")
	if riskText == "" {
		riskText = "Images contain vulnerabilities and missing environment configuration can make runtime execution fail."
	}

	return models.DashboardSpec{
		Version: models.FinalOutputSpecVersion,
		Title:   "Docker Image Operations Overview",
		Blocks: []models.DashboardBlock{
			{
				Type:  "properties",
				Title: "Overview",
				Items: []models.DashboardBlockItem{
					{Label: "Images Built", Value: dashboardNumberValue(total), Text: "Pipeline completed"},
					{Label: "Total Build Time", Value: dashboardSecondsValue(metrics.TotalSeconds), Text: "Average " + dashboardSecondsValue(metrics.AverageSeconds)},
					{Label: "Production Ready", Value: dashboardRatioValue(productionReady, total), Text: dashboardBlockedSummary(blocked)},
					{Label: "Configuration Present", Value: dashboardRatioValue(configPresent, total), Text: dashboardMissingConfigurationSummary(missingRuntime)},
				},
			},
			{
				Type:  "chart",
				Title: "Build Duration",
				Text:  "Seconds required to build each image.",
				Chart: &models.DashboardChart{
					Type: "bar",
					Unit: "s",
					Series: []models.DashboardChartSeries{
						{
							Key:    "build_duration_seconds",
							Label:  "Build Duration",
							Unit:   "s",
							Points: dashboardBuildDurationOverviewPoints(images),
						},
					},
				},
			},
			{
				Type:  "chart",
				Title: "Production Readiness",
				Text:  "Current production readiness coverage.",
				Chart: &models.DashboardChart{
					Type: "donut",
					Series: []models.DashboardChartSeries{
						{
							Key:   "production_readiness",
							Label: "Production Readiness",
							Points: []models.DashboardSeriesPoint{
								dashboardPointValue("Production Ready", productionReady),
								dashboardPointValue("Blocked From Production", blocked),
							},
						},
					},
				},
			},
			{
				Type:  "chart",
				Title: "Runtime Configuration",
				Text:  "Runtime configuration coverage for built images.",
				Chart: &models.DashboardChart{
					Type: "donut",
					Series: []models.DashboardChartSeries{
						{
							Key:   "runtime_configuration",
							Label: "Runtime Configuration",
							Points: []models.DashboardSeriesPoint{
								dashboardPointValue("Configuration Present", configPresent),
								dashboardPointValue("Missing Runtime Configuration", missingRuntime),
							},
						},
					},
				},
			},
			{
				Type:  "callout",
				Tone:  "critical",
				Title: "Production status: blocked",
				Text: fmt.Sprintf(
					"%s Changelog updated: %s.",
					riskText,
					dashboardYesNo(changelogUpdated, true),
				),
			},
			{
				Type:    "table",
				Title:   "Readiness Matrix",
				Columns: dashboardOperationsOverviewColumns(),
				Rows:    dashboardOperationsOverviewRows(images, changelogUpdated),
			},
			{
				Type:  "list",
				Title: "Next Actions",
				Items: dashboardOperationsOverviewActions(images, vulnerable, missingRuntime),
			},
		},
	}, true
}

func dashboardCountValue(counts map[string]float64, key string, fallback float64) float64 {
	normalized := dashboardEvidenceCountKey(key)
	if value, ok := counts[normalized]; ok {
		return value
	}
	for _, alias := range dashboardEvidenceCountAliases(normalized) {
		if value, ok := counts[alias]; ok {
			return value
		}
	}
	return fallback
}

func dashboardImageBoolCount(images []pipelineFinalOutputStructuredImageEvidence, pick func(pipelineFinalOutputStructuredImageEvidence) (bool, bool)) float64 {
	var count float64
	for _, image := range images {
		value, ok := pick(image)
		if ok && value {
			count++
		}
	}
	return count
}

func pipelineFinalOutputStructuredDashboardString(evidence pipelineFinalOutputLogEvidence, key string) string {
	for _, line := range evidence.Structured {
		_, payload, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &root); err != nil {
			continue
		}
		value := dashboardEvidenceString(root[key])
		if value != "" {
			return value
		}
	}
	return ""
}

func dashboardRatioValue(value, total float64) string {
	return dashboardNumberValue(value) + " / " + dashboardNumberValue(total)
}

func dashboardNumberValue(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func dashboardBlockedSummary(blocked float64) string {
	if blocked == 0 {
		return "No images blocked"
	}
	if blocked == 1 {
		return "One image blocked"
	}
	if math.Mod(blocked, 1) == 0 {
		return fmt.Sprintf("%.0f images blocked", blocked)
	}
	return dashboardNumberValue(blocked) + " images blocked"
}

func dashboardMissingConfigurationSummary(missing float64) string {
	if missing == 0 {
		return "All images configured"
	}
	if missing == 1 {
		return "One image incomplete"
	}
	if math.Mod(missing, 1) == 0 {
		return fmt.Sprintf("%.0f images incomplete", missing)
	}
	return dashboardNumberValue(missing) + " images incomplete"
}

func dashboardBuildDurationOverviewPoints(images []pipelineFinalOutputStructuredImageEvidence) []models.DashboardSeriesPoint {
	points := make([]models.DashboardSeriesPoint, 0, len(images))
	for _, image := range images {
		if !image.HasDuration {
			continue
		}
		value := image.DurationSeconds
		points = append(points, models.DashboardSeriesPoint{Label: dashboardImageEvidenceLabel(image), Value: &value})
	}
	return points
}

func dashboardImageEvidenceLabel(image pipelineFinalOutputStructuredImageEvidence) string {
	if image.Tag != "" {
		return image.Name + ":" + image.Tag
	}
	return image.Name
}

func dashboardPointValue(label string, value float64) models.DashboardSeriesPoint {
	pointValue := value
	return models.DashboardSeriesPoint{Label: label, Value: &pointValue}
}

func dashboardOperationsOverviewColumns() []models.DashboardTableColumn {
	return []models.DashboardTableColumn{
		{Key: "image", Label: "Image"},
		{Key: "environment", Label: "Environment"},
		{Key: "vulnerabilities", Label: "Vulnerabilities"},
		{Key: "missing_config", Label: "Missing Config"},
		{Key: "production_ready", Label: "Production Ready"},
		{Key: "changelog", Label: "Changelog"},
	}
}

func dashboardOperationsOverviewRows(images []pipelineFinalOutputStructuredImageEvidence, changelogUpdated bool) []map[string]json.RawMessage {
	rows := make([]map[string]json.RawMessage, 0, len(images))
	for _, image := range images {
		rows = append(rows, map[string]json.RawMessage{
			"image":            dashboardJSONString(dashboardImageEvidenceLabel(image)),
			"environment":      dashboardJSONString(image.Environment),
			"vulnerabilities":  dashboardJSONString(dashboardYesNo(image.Vulnerable, image.HasVulnerability)),
			"missing_config":   dashboardJSONString(dashboardYesNo(image.MissingRuntime, image.HasMissingRuntime)),
			"production_ready": dashboardJSONString(dashboardYesNo(image.ProductionReady, image.HasProductionReady)),
			"changelog":        dashboardJSONString(dashboardYesNo(changelogUpdated, true)),
		})
	}
	return rows
}

func dashboardYesNo(value bool, known bool) string {
	if !known {
		return "Unknown"
	}
	if value {
		return "Yes"
	}
	return "No"
}

func dashboardOperationsOverviewActions(images []pipelineFinalOutputStructuredImageEvidence, vulnerable, missingRuntime float64) []models.DashboardBlockItem {
	items := []models.DashboardBlockItem{}
	if vulnerable > 0 {
		items = append(items, models.DashboardBlockItem{
			Text: "Remediate vulnerabilities before allowing these images into production.",
			Tone: "critical",
		})
	}
	if missingRuntime > 0 {
		items = append(items, models.DashboardBlockItem{
			Text: "Add missing runtime environment configuration for " + strings.Join(dashboardImagesWithMissingRuntime(images), ", ") + ".",
			Tone: "warning",
		})
	}
	items = append(items, models.DashboardBlockItem{
		Text: "Rerun the readiness pipeline after remediation to publish an updated dashboard.",
		Tone: "info",
	})
	return items
}

func dashboardImagesWithMissingRuntime(images []pipelineFinalOutputStructuredImageEvidence) []string {
	names := []string{}
	for _, image := range images {
		if image.HasMissingRuntime && image.MissingRuntime {
			names = append(names, dashboardImageEvidenceLabel(image))
		}
	}
	if len(names) == 0 {
		return []string{"affected images"}
	}
	return names
}

func pipelineFinalOutputRequestsBuildDurationMetrics(output pipelineFinalOutputRecord) bool {
	text := strings.ToLower(strings.Join([]string{
		output.Name,
		output.Prompt,
		output.Dashboard.EntryKey,
		output.Dashboard.Preset,
	}, " "))
	return strings.Contains(text, "build duration") && strings.Contains(text, "metric")
}

func groundDashboardBuildDurationMetricsFromEvidence(spec *models.DashboardSpec, images []pipelineFinalOutputStructuredImageEvidence) bool {
	metrics, ok := dashboardBuildDurationEvidenceMetrics(images)
	if !ok {
		return false
	}
	changed := false
	for blockIndex := range spec.Blocks {
		block := &spec.Blocks[blockIndex]
		if block.Type == "properties" {
			for itemIndex := range block.Items {
				item := &block.Items[itemIndex]
				value, ok := dashboardBuildDurationGroundedPropertyValue(item.Label, metrics)
				if !ok || item.Value == value {
					continue
				}
				item.Value = value
				changed = true
			}
		}
		if block.Chart == nil {
			continue
		}
		chartContext := strings.ToLower(strings.Join([]string{block.Label, block.Title, block.Chart.Type}, " "))
		for _, series := range block.Chart.Series {
			chartContext += " " + strings.ToLower(series.Key+" "+series.Label)
		}
		if !strings.Contains(chartContext, "duration") && !strings.Contains(chartContext, "build") {
			continue
		}
		key := "build_duration_seconds"
		label := "Build Duration"
		if len(block.Chart.Series) > 0 {
			if strings.TrimSpace(block.Chart.Series[0].Key) != "" {
				key = block.Chart.Series[0].Key
			}
			if strings.TrimSpace(block.Chart.Series[0].Label) != "" {
				label = block.Chart.Series[0].Label
			}
		}
		block.Chart.Unit = "s"
		block.Chart.Series = []models.DashboardChartSeries{
			{
				Key:    key,
				Label:  label,
				Unit:   "s",
				Points: metrics.Points,
			},
		}
		changed = true
	}
	return changed
}

type dashboardBuildDurationMetrics struct {
	TotalSeconds   float64
	AverageSeconds float64
	Fastest        pipelineFinalOutputStructuredImageEvidence
	Slowest        pipelineFinalOutputStructuredImageEvidence
	Points         []models.DashboardSeriesPoint
}

func dashboardBuildDurationEvidenceMetrics(images []pipelineFinalOutputStructuredImageEvidence) (dashboardBuildDurationMetrics, bool) {
	metrics := dashboardBuildDurationMetrics{
		Points: make([]models.DashboardSeriesPoint, 0, len(images)),
	}
	count := 0
	for _, image := range images {
		if !image.HasDuration {
			continue
		}
		value := image.DurationSeconds
		metrics.TotalSeconds += value
		if count == 0 || value < metrics.Fastest.DurationSeconds {
			metrics.Fastest = image
		}
		if count == 0 || value > metrics.Slowest.DurationSeconds {
			metrics.Slowest = image
		}
		metrics.Points = append(metrics.Points, models.DashboardSeriesPoint{Label: image.Name, Value: &value})
		count++
	}
	if count == 0 {
		return dashboardBuildDurationMetrics{}, false
	}
	metrics.AverageSeconds = metrics.TotalSeconds / float64(count)
	return metrics, true
}

func dashboardBuildDurationGroundedPropertyValue(label string, metrics dashboardBuildDurationMetrics) (string, bool) {
	normalized := dashboardEvidenceCountKey(label)
	switch {
	case strings.Contains(normalized, "total") && strings.Contains(normalized, "build") && strings.Contains(normalized, "time"):
		return dashboardSecondsValue(metrics.TotalSeconds), true
	case strings.Contains(normalized, "average") && strings.Contains(normalized, "build") && strings.Contains(normalized, "time"):
		return dashboardSecondsValue(metrics.AverageSeconds), true
	case strings.Contains(normalized, "fastest") && strings.Contains(normalized, "image"):
		return dashboardImageDurationValue(metrics.Fastest), true
	case strings.Contains(normalized, "slowest") && strings.Contains(normalized, "image"):
		return dashboardImageDurationValue(metrics.Slowest), true
	default:
		return "", false
	}
}

func dashboardSecondsValue(seconds float64) string {
	return strconv.FormatFloat(seconds, 'f', -1, 64) + "s"
}

func dashboardImageDurationValue(image pipelineFinalOutputStructuredImageEvidence) string {
	name := image.Name
	if image.Tag != "" {
		name += ":" + image.Tag
	}
	return name + " (" + dashboardSecondsValue(image.DurationSeconds) + ")"
}

func dashboardGroundedChartSeriesKey(block *models.DashboardBlock, chartType string) string {
	for _, candidate := range []string{block.Label, block.Title, chartType} {
		key := dashboardEvidenceCountKey(candidate)
		if key != "" {
			return key
		}
	}
	return "evidence"
}

func dashboardGroundedChartSeriesLabel(block *models.DashboardBlock) string {
	for _, candidate := range []string{block.Label, block.Title} {
		if strings.TrimSpace(candidate) != "" {
			return dashboardEvidenceDisplayLabel(candidate)
		}
	}
	return "Evidence"
}

func pipelineFinalOutputPromptRequestsImageEvidence(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "image") || strings.Contains(lower, "docker")
}

func dashboardSpecFromPipelineFinalOutputImageEvidence(evidence pipelineFinalOutputLogEvidence) models.DashboardSpec {
	rows := make([]map[string]json.RawMessage, 0, len(evidence.Images))
	points := make([]models.DashboardSeriesPoint, 0, len(evidence.Images))
	for _, image := range evidence.Images {
		row := map[string]json.RawMessage{
			"image":    dashboardJSONString(image.Name),
			"version":  dashboardJSONString(image.Version),
			"duration": dashboardJSONString(image.Duration),
		}
		rows = append(rows, row)
		if seconds, ok := pipelineFinalOutputDurationSeconds(image.Duration); ok {
			value := seconds
			points = append(points, models.DashboardSeriesPoint{Label: image.Name, Value: &value})
		}
	}

	primaryText := "Docker image builds are the primary subject of this pipeline."
	if pipelineFinalOutputEvidenceContainsAny(evidence.Lines, "vulnerab", "production", "environment") {
		primaryText = "Docker image builds and production readiness are the primary subject of this pipeline."
	}
	blocks := []models.DashboardBlock{
		{
			Type:  "callout",
			Tone:  pipelineFinalOutputImageEvidenceTone(evidence.Lines),
			Label: "Primary Subject",
			Text:  primaryText,
		},
		{
			Type:  "properties",
			Label: "Build Statistics",
			Items: []models.DashboardBlockItem{
				{Label: "Images Built", Value: strconv.Itoa(len(evidence.Images))},
			},
		},
		{
			Type:  "table",
			Title: "Built Images",
			Columns: []models.DashboardTableColumn{
				{Key: "image", Label: "Image Name"},
				{Key: "version", Label: "Version"},
				{Key: "duration", Label: "Build Duration"},
			},
			Rows: rows,
		},
	}
	if len(points) == len(evidence.Images) {
		blocks = append(blocks, models.DashboardBlock{
			Type:  "chart",
			Title: "Build Duration",
			Chart: &models.DashboardChart{
				Type: "bar",
				Unit: "s",
				Series: []models.DashboardChartSeries{
					{
						Key:    "build_duration_seconds",
						Label:  "Build duration",
						Unit:   "s",
						Points: points,
					},
				},
			},
		})
	}
	if followUps := pipelineFinalOutputImageEvidenceFollowUps(evidence.Lines); len(followUps) > 0 {
		blocks = append(blocks, models.DashboardBlock{
			Type:  "list",
			Title: "Important Follow-ups",
			Items: followUps,
		})
	}
	return models.DashboardSpec{
		Version: models.FinalOutputSpecVersion,
		Title:   "Image Build Summary",
		Blocks:  blocks,
	}
}

func dashboardJSONString(value string) json.RawMessage {
	payload, _ := json.Marshal(value)
	return payload
}

func pipelineFinalOutputImageEvidenceTone(lines []string) string {
	if pipelineFinalOutputEvidenceContainsAny(lines, "vulnerab", "can not be run in production", "cannot be run in production", "fails during running", "missing") {
		return "warning"
	}
	return "info"
}

func pipelineFinalOutputImageEvidenceFollowUps(lines []string) []models.DashboardBlockItem {
	items := []models.DashboardBlockItem{}
	for _, line := range lines {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "vulnerab"):
			items = append(items, models.DashboardBlockItem{Text: strings.TrimSpace(line), Tone: "warning"})
		case strings.Contains(lower, "environment") && (strings.Contains(lower, "not provided") || strings.Contains(lower, "fail")):
			items = append(items, models.DashboardBlockItem{Text: strings.TrimSpace(line), Tone: "warning"})
		case strings.Contains(lower, "changelog"):
			items = append(items, models.DashboardBlockItem{Text: strings.TrimSpace(line), Tone: "success"})
		}
	}
	return items
}

func pipelineFinalOutputEvidenceContainsAny(lines []string, fragments ...string) bool {
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, fragment := range fragments {
			if strings.Contains(lower, strings.ToLower(fragment)) {
				return true
			}
		}
	}
	return false
}

func pipelineFinalOutputDurationSeconds(raw string) (float64, bool) {
	match := pipelineFinalOutputDurationTokenPattern.FindStringSubmatch(raw)
	if len(match) == 0 {
		return 0, false
	}
	token := strings.ToLower(strings.Join(strings.Fields(match[0]), ""))
	unitStart := len(token)
	for index, r := range token {
		if (r < '0' || r > '9') && r != '.' {
			unitStart = index
			break
		}
	}
	if unitStart <= 0 || unitStart >= len(token) {
		return 0, false
	}
	value, err := strconv.ParseFloat(token[:unitStart], 64)
	if err != nil {
		return 0, false
	}
	switch token[unitStart:] {
	case "ms":
		return value / 1000, true
	case "s", "sec", "secs", "second", "seconds":
		return value, true
	case "m", "min", "mins", "minute", "minutes":
		return value * 60, true
	case "h", "hr", "hrs", "hour", "hours":
		return value * 3600, true
	default:
		return 0, false
	}
}

func mustMarshalDashboardTarget(target models.DashboardOutputTarget) string {
	payload, err := json.Marshal(target)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func (a *App) recordPipelineFinalOutputAttemptUsage(
	ctx context.Context,
	runID string,
	output pipelineFinalOutputRecord,
	result pipelineFinalOutputGenerationResult,
) {
	for _, report := range pipelineFinalOutputAttemptUsageReports(output, result) {
		_ = a.recordAIUsage(ctx, runID, report)
	}
}

func pipelineFinalOutputAttemptUsageReports(
	output pipelineFinalOutputRecord,
	result pipelineFinalOutputGenerationResult,
) []models.AIUsageReport {
	reports := make([]models.AIUsageReport, 0, len(result.Attempts))
	for index, attempt := range result.Attempts {
		usage := attempt.Completion.Usage
		if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
			continue
		}
		reports = append(reports, models.AIUsageReport{
			Feature:          pipelineFinalOutputFeature,
			Provider:         usage.Provider,
			Model:            usage.Model,
			LLMProfile:       usage.Profile,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Metadata: map[string]any{
				"output_id":      output.ID,
				"output_name":    output.Name,
				"output_type":    output.Type,
				"estimated":      usage.Estimated,
				"attempt":        index + 1,
				"retry":          index > 0,
				"contract_valid": attempt.ContractValid,
			},
		})
	}
	return reports
}

func (a *App) markPipelineFinalOutputFailure(ctx context.Context, outputID string, cause error) error {
	return a.markPipelineFinalOutputFailureWithResult(
		ctx,
		outputID,
		cause,
		pipelineFinalOutputGenerationResult{},
	)
}

func (a *App) markPipelineFinalOutputFailureWithResult(
	ctx context.Context,
	outputID string,
	cause error,
	result pipelineFinalOutputGenerationResult,
) error {
	message := ""
	if cause != nil {
		message = strings.TrimSpace(cause.Error())
	}
	if message == "" {
		message = "failed to generate final output"
	}
	attempts := 0
	violations := 0
	attempts = len(result.Attempts)
	violations = result.ContractViolations
	_, err := a.db.Exec(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'failure', error = $2,
		    generation_attempts = $3, contract_violations = $4, updated_at = NOW()
		WHERE id = $1 AND status <> 'cancelled'
	`, outputID, message, attempts, violations)
	return err
}

func (a *App) pipelineFinalOutputProfiles(ctx context.Context) (string, map[string]config.LLMProfile, error) {
	if a != nil && a.db != nil {
		defaultProfile, profiles, found, err := a.loadLLMProfilesFromDB(ctx)
		if err != nil {
			return "", nil, err
		}
		if found {
			return defaultProfile, profiles, nil
		}
	}
	if a == nil || a.cfg == nil {
		return "", nil, nil
	}
	defaultProfile, profiles := a.llmProfilesSnapshot()
	return defaultProfile, profiles, nil
}

func (a *App) pipelineFinalOutputLLMClient(ctx context.Context, profileName, scope string) (*llmclient.Client, error) {
	defaultProfile, profiles, err := a.pipelineFinalOutputProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profileName = config.NormalizeLLMProfileName(profileName)
	if profileName == "" {
		profileName = config.NormalizeLLMProfileName(defaultProfile)
	}
	if profileName == "" {
		return nil, fmt.Errorf("no LLM profile is configured for final output generation")
	}
	profile, ok := profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("LLM profile %q is not configured", profileName)
	}
	profile = config.NormalizeLLMProfile(profile)
	if !config.LLMProfileAllowedInScope(profile, scope) {
		return nil, fmt.Errorf("LLM profile %q is not allowed in scope %q", profileName, strings.Trim(strings.TrimSpace(scope), "/"))
	}
	if status, message := a.validateLLMProfileConfiguration(ctx, profileName, profile); status != "valid" {
		message = strings.TrimSpace(message)
		if message == "" {
			message = fmt.Sprintf("LLM profile %q is invalid", profileName)
		}
		return nil, errors.New(message)
	}
	apiKey := ""
	if config.LLMProviderRequiresAPIKey(profile.Provider) {
		value, err := a.resolveLLMProfileAPIKey(ctx, profileName, profile)
		if err != nil {
			return nil, err
		}
		apiKey = value
	}
	zeroTemperature := 0.0
	return llmclient.New(llmclient.Options{
		Provider:       profile.Provider,
		Profile:        profileName,
		APIKey:         apiKey,
		Model:          profile.Model,
		BaseURL:        config.EffectiveLLMProfileBaseURL(profile),
		Reasoning:      config.EffectiveLLMProfileReasoning(profile),
		TimeoutSeconds: profile.TimeoutSeconds,
		MaxTokens:      profile.MaxTokens,
		Temperature:    &zeroTemperature,
		Extra:          cloneStringMap(profile.Extra),
		HTTPClient:     assistantHTTPClient(a),
	}), nil
}

func (a *App) buildPipelineFinalOutputRunContext(ctx context.Context, runID string) (pipelineFinalOutputRunContext, error) {
	record, err := runquery.LoadRunRecord(ctx, a.db, runID)
	if err != nil {
		return pipelineFinalOutputRunContext{}, err
	}
	tasksByStep, err := runquery.LoadTaskDetailsByStep(ctx, a.db, runID)
	if err != nil {
		return pipelineFinalOutputRunContext{}, err
	}
	childRuns, err := runquery.LoadChildRuns(ctx, a.db, runID)
	if err != nil {
		return pipelineFinalOutputRunContext{}, err
	}
	history, err := a.loadPipelineFinalOutputRunHistory(ctx, runID, record.Run)
	if err != nil {
		return pipelineFinalOutputRunContext{}, err
	}
	logs, err := a.loadPipelineFinalOutputLogExcerpt(ctx, runID)
	if err != nil {
		return pipelineFinalOutputRunContext{}, err
	}
	logEvidence := buildPipelineFinalOutputLogEvidence(logs)

	var pipeline models.Pipeline
	_ = yaml.Unmarshal([]byte(record.PipelineDefinitionYAML), &pipeline)
	run := runquery.ApplyChildRunStatus(record.Run, childRuns)
	scope := a.pipelineFinalOutputRunScope(ctx, runID)
	stepDetails := runquery.BuildStepDetailsForRun(run, pipeline, pipeline, tasksByStep, childRuns, nil, nil)

	var builder strings.Builder
	builder.WriteString("Pipeline run summary\n")
	writeFinalOutputLine(&builder, "Run ID", run.RunID)
	writeFinalOutputLine(&builder, "Pipeline", strings.Trim(strings.TrimSpace(run.PipelinePath+"/"+run.PipelineName), "/"))
	writeFinalOutputLine(&builder, "Version", run.PipelineVersion)
	writeFinalOutputLine(&builder, "Status", run.Status)
	writeFinalOutputLine(&builder, "Duration", run.Duration)
	writeFinalOutputLine(&builder, "Scope", scope)
	writeFinalOutputLine(&builder, "Repository", strings.Trim(strings.TrimSpace(run.GitRepoOwner+"/"+run.GitRepoName), "/"))
	writeFinalOutputLine(&builder, "Branch", firstNonEmptyString(run.GitRef, run.GitTargetRef))
	writeFinalOutputLine(&builder, "Commit", run.GitCommitSHA)
	writeFinalOutputLine(&builder, "Failure reason", run.FailureReason)
	writeFinalOutputTime(&builder, "Started at", run.StartedAt)
	writeFinalOutputTime(&builder, "Finished at", run.FinishedAt)

	writePipelineFinalOutputCurrentLogEvidence(&builder, logs, logEvidence)
	writePipelineFinalOutputRunHistory(&builder, history)

	builder.WriteString("\nSteps and tasks\n")
	if len(stepDetails) > 0 {
		for _, step := range stepDetails {
			stepName := strings.TrimSpace(step.Name)
			if stepName == "" {
				stepName = "unknown"
			}
			fmt.Fprintf(&builder, "- Step: %s | status: %s", stepName, step.Status)
			if strings.TrimSpace(step.Duration) != "" {
				fmt.Fprintf(&builder, " | duration: %s", step.Duration)
			}
			builder.WriteString("\n")
			for _, task := range step.Tasks {
				fmt.Fprintf(&builder, "  - Task: %s | status: %s", task.TaskName, task.Status)
				if task.ExitCode != nil {
					fmt.Fprintf(&builder, " | exit_code: %d", *task.ExitCode)
				}
				if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
					fmt.Fprintf(&builder, " | duration: %s", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
				}
				builder.WriteString("\n")
			}
		}
	} else {
		for stepName, tasks := range tasksByStep {
			fmt.Fprintf(&builder, "- Step: %s\n", stepName)
			for _, task := range tasks {
				fmt.Fprintf(&builder, "  - Task: %s | status: %s\n", task.TaskName, task.Status)
			}
		}
	}

	if len(childRuns) > 0 {
		builder.WriteString("\nChild runs\n")
		for _, child := range childRuns {
			fmt.Fprintf(&builder, "- %s | pipeline: %s | step: %s | status: %s\n", child.RunID, child.PipelineName, child.ParentStepName, child.Status)
		}
	}

	return pipelineFinalOutputRunContext{Text: builder.String(), Scope: scope, LogEvidence: logEvidence}, nil
}

func (a *App) loadPipelineFinalOutputRunHistory(ctx context.Context, currentRunID string, currentRun models.RunListItem) ([]models.RunListItem, error) {
	pipelineName := strings.TrimSpace(currentRun.PipelineName)
	if pipelineName == "" {
		return nil, nil
	}
	pipelinePath := strings.TrimSpace(currentRun.PipelinePath)
	rows, err := a.db.Query(ctx, `
		SELECT run_id::text, pipeline_name, COALESCE(pipeline_path, ''), COALESCE(pipeline_version, ''),
		       status, started_at, finished_at, COALESCE(failure_reason, '')
		FROM pipeline_runs
		WHERE run_id::text <> $1
		  AND COALESCE(pipeline_path, '') = $2
		  AND pipeline_name = $3
		ORDER BY created_at DESC
		LIMIT $4
	`, currentRunID, pipelinePath, pipelineName, pipelineFinalOutputHistoryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := []models.RunListItem{}
	for rows.Next() {
		var item models.RunListItem
		var startedAt, finishedAt sql.NullTime
		var pipelineVersion string
		if err := rows.Scan(
			&item.RunID,
			&item.PipelineName,
			&item.PipelinePath,
			&pipelineVersion,
			&item.Status,
			&startedAt,
			&finishedAt,
			&item.FailureReason,
		); err != nil {
			return nil, err
		}
		item.PipelineVersion = runquery.NormalizePipelineVersion(pipelineVersion)
		if startedAt.Valid {
			item.StartedAt = startedAt.Time
			if finishedAt.Valid {
				item.FinishedAt = finishedAt.Time
				item.Duration = item.FinishedAt.Sub(item.StartedAt).Round(time.Second).String()
				item.IsComplete = true
			} else {
				item.Duration = time.Since(item.StartedAt).Round(time.Second).String()
				item.IsComplete = runquery.IsTerminalRunStatus(item.Status)
			}
		} else {
			item.IsComplete = runquery.IsTerminalRunStatus(item.Status)
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func (a *App) pipelineFinalOutputRunScope(ctx context.Context, runID string) string {
	var scope sql.NullString
	if err := a.db.QueryRow(ctx, `SELECT COALESCE(scope, '') FROM pipeline_runs WHERE run_id = $1`, runID).Scan(&scope); err != nil {
		return ""
	}
	return scope.String
}

func (a *App) loadPipelineFinalOutputLogExcerpt(ctx context.Context, runID string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT line
		FROM pipeline_run_logs
		WHERE run_id = $1
		ORDER BY id DESC
		LIMIT 120
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reversed := []string{}
	totalBytes := 0
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		totalBytes += len(line)
		if totalBytes > 24000 {
			break
		}
		reversed = append(reversed, line)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

func writePipelineFinalOutputRunHistory(builder *strings.Builder, history []models.RunListItem) {
	if len(history) == 0 {
		return
	}
	builder.WriteString("\nRecent pipeline history\n")
	for _, item := range history {
		runID := strings.TrimSpace(item.RunID)
		if runID == "" {
			runID = "unknown"
		}
		fmt.Fprintf(builder, "- %s", runID)
		pipeline := strings.Trim(strings.TrimSpace(item.PipelinePath+"/"+item.PipelineName), "/")
		if pipeline != "" {
			fmt.Fprintf(builder, " | pipeline: %s", pipeline)
		}
		if version := strings.TrimSpace(item.PipelineVersion); version != "" {
			fmt.Fprintf(builder, " | version: %s", version)
		}
		if status := strings.TrimSpace(item.Status); status != "" {
			fmt.Fprintf(builder, " | status: %s", status)
		}
		if duration := strings.TrimSpace(item.Duration); duration != "" {
			fmt.Fprintf(builder, " | duration: %s", duration)
		}
		writeFinalOutputTimeFragment(builder, "started_at", item.StartedAt)
		writeFinalOutputTimeFragment(builder, "finished_at", item.FinishedAt)
		if reason := strings.TrimSpace(item.FailureReason); reason != "" {
			fmt.Fprintf(builder, " | failure_reason: %s", reason)
		}
		builder.WriteString("\n")
	}
}

func writePipelineFinalOutputCurrentLogEvidence(builder *strings.Builder, logs []string, evidence pipelineFinalOutputLogEvidence) {
	if len(logs) == 0 {
		return
	}
	builder.WriteString("\nCurrent run emitted evidence (authoritative for business facts)\n")
	builder.WriteString("Use emitted step output before operational runner logs, metadata, history, or configured runtime/container fields when answering questions about produced business data.\n")
	if summary := pipelineFinalOutputLogEvidenceSummaryFromEvidence(evidence); len(summary) > 0 {
		builder.WriteString("Extracted facts from current log lines\n")
		for _, line := range summary {
			fmt.Fprintf(builder, "- %s\n", line)
		}
	}
	if len(evidence.Structured) > 0 {
		builder.WriteString("Structured emitted evidence\n")
		for _, line := range evidence.Structured {
			fmt.Fprintf(builder, "- %s\n", line)
		}
	}
	if len(evidence.Lines) > 0 {
		builder.WriteString("Emitted step output lines\n")
		for _, line := range evidence.Lines {
			fmt.Fprintf(builder, "- %s\n", line)
		}
	}
	builder.WriteString("Raw operational log excerpt (use only for run status and operational metadata unless it contains emitted step output)\n")
	for _, line := range logs {
		fmt.Fprintf(builder, "- %s\n", line)
	}
}

type pipelineFinalOutputImageEvidence struct {
	Name     string
	Version  string
	Duration string
}

func pipelineFinalOutputLogEvidenceSummary(logs []string) []string {
	return pipelineFinalOutputLogEvidenceSummaryFromEvidence(buildPipelineFinalOutputLogEvidence(logs))
}

func pipelineFinalOutputLogEvidenceSummaryFromEvidence(evidence pipelineFinalOutputLogEvidence) []string {
	if len(evidence.Images) == 0 {
		return nil
	}
	summary := []string{fmt.Sprintf("image_count: %d", len(evidence.Images))}
	for _, image := range evidence.Images {
		line := fmt.Sprintf("image: %s | version: %s", image.Name, image.Version)
		if image.Duration != "" {
			line += " | build_duration: " + image.Duration
		}
		summary = append(summary, line)
	}
	return summary
}

func buildPipelineFinalOutputLogEvidence(logs []string) pipelineFinalOutputLogEvidence {
	lines := pipelineFinalOutputEmittedEvidenceLines(logs)
	if len(lines) == 0 {
		lines = pipelineFinalOutputFallbackEvidenceLines(logs)
	}
	return pipelineFinalOutputLogEvidence{
		Lines:      lines,
		Structured: pipelineFinalOutputStructuredEvidenceFromLines(lines),
		Images:     pipelineFinalOutputImageEvidenceFromLines(lines),
	}
}

func pipelineFinalOutputStructuredEvidenceFromLines(lines []string) []string {
	evidence := make([]string, 0)
	totalBytes := 0
	for _, line := range lines {
		key, payload, ok := pipelineFinalOutputStructuredEvidenceLine(line)
		if !ok {
			continue
		}
		entry := key + "=" + payload
		totalBytes += len(entry)
		if totalBytes > 12000 {
			break
		}
		evidence = append(evidence, entry)
		if len(evidence) >= 8 {
			break
		}
	}
	return evidence
}

func pipelineFinalOutputStructuredEvidenceLine(line string) (string, string, bool) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if !pipelineFinalOutputEvidenceKeyPattern.MatchString(key) {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return "", "", false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(value)); err != nil {
		return "", "", false
	}
	return key, compact.String(), true
}

func pipelineFinalOutputImageEvidenceFromLines(lines []string) []pipelineFinalOutputImageEvidence {
	images := []pipelineFinalOutputImageEvidence{}
	seenImages := map[string]struct{}{}
	for _, line := range lines {
		if !pipelineFinalOutputLooksLikeImageEvidenceLine(line) {
			continue
		}
		for _, ref := range pipelineFinalOutputImageRefPattern.FindAllString(line, -1) {
			if strings.Contains(ref, "://") {
				continue
			}
			separator := strings.LastIndex(ref, ":")
			if separator <= 0 || separator == len(ref)-1 {
				continue
			}
			image := pipelineFinalOutputImageEvidence{
				Name:    strings.TrimSpace(ref[:separator]),
				Version: strings.TrimSpace(ref[separator+1:]),
			}
			if image.Name == "" || image.Version == "" {
				continue
			}
			key := image.Name + ":" + image.Version
			if _, ok := seenImages[key]; ok {
				continue
			}
			seenImages[key] = struct{}{}
			images = append(images, image)
		}
	}
	if len(images) == 0 {
		return nil
	}

	durations := []string{}
	for _, line := range lines {
		if !pipelineFinalOutputLooksLikeBuildDurationLine(line) {
			continue
		}
		for _, duration := range pipelineFinalOutputDurationTokenPattern.FindAllString(line, -1) {
			durations = append(durations, strings.Join(strings.Fields(duration), ""))
		}
	}
	for index := range images {
		if index < len(durations) {
			images[index].Duration = durations[index]
		}
	}
	return images
}

func pipelineFinalOutputEmittedEvidenceLines(logs []string) []string {
	lines := []string{}
	for _, line := range logs {
		message := pipelineFinalOutputLogMessage(line)
		outputLines := pipelineFinalOutputCommandOutputLines(message)
		lines = append(lines, outputLines...)
	}
	return compactNonEmptyStrings(lines)
}

func pipelineFinalOutputFallbackEvidenceLines(logs []string) []string {
	lines := []string{}
	for _, line := range logs {
		message := pipelineFinalOutputLogMessage(line)
		if pipelineFinalOutputLooksLikeOperationalLogLine(message) {
			continue
		}
		lines = append(lines, message)
	}
	return compactNonEmptyStrings(lines)
}

func pipelineFinalOutputLogMessage(line string) string {
	line = strings.TrimSpace(line)
	jsonStart := strings.Index(line, "{")
	if jsonStart < 0 {
		return line
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line[jsonStart:]), &payload); err != nil {
		return line
	}
	var message string
	if err := json.Unmarshal(payload["message"], &message); err != nil || strings.TrimSpace(message) == "" {
		return line
	}
	return strings.TrimSpace(message)
}

func pipelineFinalOutputCommandOutputLines(message string) []string {
	index := strings.LastIndex(message, "output=")
	if index < 0 {
		return nil
	}
	tail := strings.TrimSpace(message[index+len("output="):])
	value, ok := pipelineFinalOutputTrailingQuotedAssignmentValue(tail)
	if !ok {
		value, ok = pipelineFinalOutputQuotedAssignmentValue(tail)
	}
	if !ok {
		value, ok = pipelineFinalOutputEscapedQuotedAssignmentValue(tail)
	}
	if !ok {
		return nil
	}
	return splitPipelineFinalOutputEvidenceLines(value)
}

func pipelineFinalOutputTrailingQuotedAssignmentValue(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	return decodePipelineFinalOutputAssignmentEscapes(raw[1 : len(raw)-1]), true
}

func decodePipelineFinalOutputAssignmentEscapes(raw string) string {
	var builder *strings.Builder
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		if ch != '\\' || index+1 >= len(raw) {
			if builder != nil {
				builder.WriteByte(ch)
			}
			continue
		}
		next := raw[index+1]
		var decoded byte
		switch next {
		case 'n':
			decoded = '\n'
		case 'r':
			decoded = '\r'
		case 't':
			decoded = '\t'
		case '\\':
			decoded = '\\'
		case '"':
			decoded = '"'
		default:
			if builder != nil {
				builder.WriteByte(ch)
				builder.WriteByte(next)
			}
			index++
			continue
		}
		if builder == nil {
			builder = &strings.Builder{}
			builder.Grow(len(raw))
			builder.WriteString(raw[:index])
		}
		builder.WriteByte(decoded)
		index++
	}
	if builder == nil {
		return raw
	}
	return builder.String()
}

func pipelineFinalOutputQuotedAssignmentValue(raw string) (string, bool) {
	if raw == "" || raw[0] != '"' {
		return "", false
	}
	var builder strings.Builder
	escaped := false
	for index := 1; index < len(raw); index++ {
		ch := raw[index]
		if escaped {
			switch ch {
			case 'n':
				builder.WriteByte('\n')
			case 'r':
				builder.WriteByte('\r')
			case 't':
				builder.WriteByte('\t')
			default:
				builder.WriteByte(ch)
			}
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '"':
			return builder.String(), true
		default:
			builder.WriteByte(ch)
		}
	}
	return "", false
}

func pipelineFinalOutputEscapedQuotedAssignmentValue(raw string) (string, bool) {
	if !strings.HasPrefix(raw, `\"`) {
		return "", false
	}
	var builder strings.Builder
	for index := 2; index < len(raw); index++ {
		ch := raw[index]
		if ch != '\\' {
			builder.WriteByte(ch)
			continue
		}
		if index+1 >= len(raw) {
			return "", false
		}
		next := raw[index+1]
		if next == '"' {
			return builder.String(), true
		}
		switch next {
		case 'n':
			builder.WriteByte('\n')
		case 'r':
			builder.WriteByte('\r')
		case 't':
			builder.WriteByte('\t')
		case '\\':
			builder.WriteByte('\\')
		default:
			builder.WriteByte(next)
		}
		index++
	}
	return "", false
}

func splitPipelineFinalOutputEvidenceLines(value string) []string {
	return compactNonEmptyStrings(strings.Split(value, "\n"))
}

func compactNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func pipelineFinalOutputLooksLikeImageEvidenceLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "image") || strings.Contains(lower, "docker") || strings.Contains(lower, "container") || strings.Contains(lower, "artifact")
}

func pipelineFinalOutputLooksLikeOperationalLogLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return true
	}
	for _, prefix := range []string{
		"trigger event id:",
		"preparing agent container",
		"assigned pipeline ",
		"dispatched to runner",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, fragment := range []string{
		"pipeline execution starting",
		"agent starting with embedded",
		"starting asynchronous image pre-pull",
		"creating new container for step",
		"image found locally",
		"executing direct script",
		"successfully notified dispatcher",
		"cleaning up session container",
		"cleaning up pipeline container",
		"pipeline finished successfully",
	} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func pipelineFinalOutputLooksLikeBuildDurationLine(line string) bool {
	lower := strings.ToLower(line)
	hasDurationSignal := strings.Contains(lower, "duration") ||
		strings.Contains(lower, "took") ||
		strings.Contains(lower, "time")
	hasBuildSignal := strings.Contains(lower, "build") ||
		strings.Contains(lower, "built") ||
		strings.Contains(lower, "image") ||
		strings.Contains(lower, "artifact")
	return hasDurationSignal && hasBuildSignal
}

func buildPipelineFinalOutputPrompt(runContext string, output pipelineFinalOutputRecord) string {
	var builder strings.Builder
	builder.WriteString("You are creating a polished final deliverable for an enterprise pipeline run.\n")
	builder.WriteString("Use the full run context below, but do not expose secrets, credentials, tokens, or raw environment variable values. Do copy non-secret operational labels from emitted evidence exactly, including image tags, environment names, versions, statuses, and JSON field names.\n")
	builder.WriteString("Treat emitted current-run step output, including structured JSON, NDJSON, and plain-language log lines, as the primary source for business facts. If emitted step output contains values that answer the user instruction, copy those values exactly.\n")
	builder.WriteString("Do not substitute configured container images, runner/runtime images, LLM/agent metadata, operational log image-pull lines, or recent-history values for business entities such as images, versions, artifacts, services, or subjects unless the emitted step output explicitly identifies them as the requested entities.\n")
	builder.WriteString("If a file is mentioned but its contents are not present in the run context, do not infer or invent its values. Prefer operationally relevant subjects over incidental personal/noise lines unless the user specifically asks for those details.\n")
	builder.WriteString("For intent-level dashboard requests, infer the dashboard structure from the requested facts and available evidence. Keep the dashboard scoped to the user instruction; do not add generic run metadata, configured runtime/container details, or incidental facts unless requested. Use run history, step, and task duration metadata for operational run timing only when current log evidence does not answer the requested business timing.\n")
	builder.WriteString("The system output contract defines the required response envelope.\n\n")
	fmt.Fprintf(&builder, "Output name: %s\n", output.Name)
	fmt.Fprintf(&builder, "Output type: %s\n", output.Type)
	if normalizePipelineFinalOutputType(output.Type) == "dashboard" {
		writeFinalOutputLine(&builder, "Dashboard ref", output.Dashboard.Ref)
		writeFinalOutputLine(&builder, "Dashboard section", output.Dashboard.Section)
		writeFinalOutputLine(&builder, "Dashboard entry key", output.Dashboard.EntryKey)
		writeFinalOutputLine(&builder, "Dashboard publish mode", output.Dashboard.Mode)
		writeFinalOutputLine(&builder, "Dashboard preset", output.Dashboard.Preset)
	}
	fmt.Fprintf(&builder, "Format requirements: %s\n\n", pipelineFinalOutputFormatGuidance(output))
	builder.WriteString("User instruction:\n")
	fmt.Fprintf(&builder, "%s\n\n", strings.TrimSpace(output.Prompt))
	builder.WriteString("Run context:\n")
	builder.WriteString(runContext)
	return builder.String()
}

func pipelineFinalOutputFormatGuidance(output pipelineFinalOutputRecord) string {
	switch normalizePipelineFinalOutputType(output.Type) {
	case "markdown":
		return "Inside <final_output>, provide clean Markdown suitable for preview and copy."
	case "pdf":
		return pipelineDocumentSpecGuidance("PDF")
	case "excel":
		return `Inside <final_output>, provide only a SpreadsheetSpec JSON object. Use {"version":"1","title":"...","sheets":[{"name":"Summary","columns":[{"key":"name","header":"Name","width":24,"number_format":"text"}],"rows":[{"name":"Example"}],"freeze_header":true,"auto_filter":true}]}. Column keys must start with a letter and contain only letters, numbers, or underscores. Cell values must be JSON strings, numbers, booleans, or null. Supported number_format values are text, integer, decimal, percent, currency_usd, currency_eur, date, datetime, and boolean.`
	case "json":
		return "Inside <final_output>, provide valid JSON without Markdown fences or commentary."
	case "html":
		return pipelineDocumentSpecGuidance("HTML")
	case "dashboard":
		return dashboardFinalOutputFormatGuidanceForTarget(output.Dashboard)
	default:
		return "Inside <final_output>, provide concise business-readable text."
	}
}

func dashboardFinalOutputFormatGuidance(preset string) string {
	base := `Inside <final_output>, provide only a valid DashboardSpec JSON object. Translate the user's dashboard intent into a useful dashboard from the run context; do not require the user to know schema details. Choose the dashboard structure dynamically from the prompt, pipeline definition, run metadata, recent pipeline history, step/task durations, child runs, and log evidence. If the user did not name a visualization, choose by data shape: text or callout for narrative conclusions, status/progress/properties for current state and scalar facts, table for repeated records, bar chart for categorical counts/durations/rankings, line or area chart for time series, and pie or donut chart only for bounded part-to-whole data. Use only evidence present in the run context; if requested data is absent, say it is not present rather than guessing. Keep content scoped to the user's dashboard request and avoid generic run metadata unless requested. Copy non-secret operational labels from emitted evidence exactly, including tags such as prod, environment names such as production, versions, statuses, and JSON field names. Available DashboardSpec blocks are status, text, callout, list, properties, table, progress, link, chart, and series. Include a non-empty title. Use one flat top-level blocks array; do not wrap dashboard output in sections or widgets, and do not put nested blocks or widgets inside a block. Use text for text and callout block bodies. Use label for display labels; key is only for table columns and chart series identifiers. Tables need columns with key/label and scalar row values. Charts need type, series, and points with label or timestamp plus finite numeric value; put units in chart.unit or chart.series[].unit and use chart.type values such as line, bar, area, pie, or donut instead of a shape field. Do not include Markdown, HTML, CSS, JavaScript, commentary, or unsafe links. The response is validated before publication and will be retried if it does not match the DashboardSpec contract.`
	if presetGuidance := dashboardFinalOutputPresetGuidance(preset); presetGuidance != "" {
		return base + " Preset guidance: " + presetGuidance
	}
	return base
}

func dashboardFinalOutputFormatGuidanceForTarget(target models.DashboardOutputTarget) string {
	guidance := dashboardFinalOutputFormatGuidance(target.Preset)
	if modeGuidance := dashboardFinalOutputPublishModeGuidance(target.Mode); modeGuidance != "" {
		guidance += " Publication mode guidance: " + modeGuidance
	}
	return guidance
}

func dashboardFinalOutputPublishModeGuidance(mode string) string {
	switch normalizeDashboardPublishMode(mode) {
	case dashboardPublishModeSeries:
		return "Because dashboard publish mode is series, include at least one chart or series block with a chart object, chart.series array, and chart.series[].points. Use line, bar, or area charts for series blocks; chart blocks may also use pie or donut for bounded part-to-whole metrics."
	default:
		return ""
	}
}

func dashboardFinalOutputPresetGuidance(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "report":
		return "report means a narrative operator report. Start with a text executive summary or callout, then short titled text or list blocks for what changed, blockers or risks, and the next action. Tables are allowed only as compact supporting evidence after the narrative; do not make a table the primary or first block unless the user explicitly asks for a report appendix."
	case "table":
		return "table means a row-and-column output. Make one table the primary block, use clear columns with stable keys for repeated records, and add at most one short status or callout summary before it. Avoid charts or long narrative unless the user explicitly asks for them."
	case "status":
		return "status means current health or readiness. Start with one status block or callout that states the overall condition, then use properties, progress, or a short list for attention items and next checks. Avoid large tables unless the status depends on repeated records."
	case "timeline":
		return "timeline means chronological order. Use a series line or area chart for timestamped numeric data, or a titled list/table sorted oldest-to-newest for discrete events. Include timestamps or ordered labels and avoid side-by-side comparison unless the user asks for it."
	case "comparison":
		return "comparison means side-by-side differences. Use a comparison table or properties blocks keyed by the compared services, environments, versions, or options, and include a short callout for the most important difference, winner, or risk. Charts are optional only for numeric comparisons."
	case "metrics":
		return "metrics means numbers first. Start with properties or status blocks for headline values, include units and ratios, and use bar charts for categorical metrics or line/area/series charts for trends. Tables are supporting detail only when exact per-entity metric rows are requested."
	case "mixed", "auto":
		if strings.EqualFold(strings.TrimSpace(preset), "mixed") {
			return "mixed means a cohesive multi-block digest. Combine complementary blocks in this order when useful: headline properties/status, charts, risk callouts, tables, and next-action lists. Each block should answer a distinct operator question without duplicating the same facts."
		}
		return "auto means choose the smallest useful presentation for the requested facts and evidence. Prefer one or two blocks when the answer is simple, and escalate to table, chart, report, or mixed layout only when the prompt or data shape calls for it."
	default:
		return ""
	}
}

func pipelineDocumentSpecGuidance(target string) string {
	return `Inside <final_output>, provide only a DocumentSpec JSON object for ` + target + `. ` +
		`Use {"version":"1","title":"...","subtitle":"...","metadata":[{"label":"Run","value":"..."}],` +
		`"sections":[{"title":"Summary","blocks":[...]}]}. ` +
		`Supported blocks are {"type":"paragraph","text":"..."}, ` +
		`{"type":"bullet_list","items":["..."]}, {"type":"numbered_list","items":["..."]}, ` +
		`{"type":"table","table":{"columns":["..."],"rows":[["..."]]}}, and ` +
		`{"type":"callout","title":"...","tone":"info|success|warning|critical","text":"..."}. ` +
		`Every section requires a title and at least one block. Do not include Markdown or HTML.`
}

func resolvePipelineFinalOutputProfileName(defaultProfile string, pipeline models.Pipeline, item models.PipelineOutputItem) string {
	return config.NormalizeLLMProfileName(firstNonEmptyString(
		item.LLMProfile,
		pipeline.Output.LLMProfile,
		pipeline.LLMProfile,
		defaultProfile,
	))
}

func pipelineFinalOutputMatchesRunStatus(when, runStatus string) bool {
	switch normalizePipelineFinalOutputWhen(when) {
	case "always":
		return true
	case "success":
		return runquery.NormalizeRunDetailStatus(runStatus) == "success"
	case "failure":
		normalized := runquery.NormalizeRunDetailStatus(runStatus)
		return runquery.IsTerminalRunStatus(normalized) && normalized != "success"
	default:
		return false
	}
}

func normalizePipelineFinalOutputWhen(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "always":
		return "always"
	case "success", "succeeded":
		return "success"
	case "failure", "failed", "error":
		return "failure"
	default:
		return ""
	}
}

func normalizePipelineFinalOutputType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "markdown", "md":
		return "markdown"
	case "pdf":
		return "pdf"
	case "excel", "xlsx", "xls", "spreadsheet":
		return "excel"
	case "json":
		return "json"
	case "html":
		return "html"
	case "dashboard", "dash", "dashboard_spec":
		return "dashboard"
	default:
		return ""
	}
}

func writeFinalOutputLine(builder *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(builder, "%s: %s\n", label, value)
}

func writeFinalOutputTime(builder *strings.Builder, label string, value time.Time) {
	if value.IsZero() {
		return
	}
	fmt.Fprintf(builder, "%s: %s\n", label, value.UTC().Format(time.RFC3339))
}

func writeFinalOutputTimeFragment(builder *strings.Builder, label string, value time.Time) {
	if value.IsZero() {
		return
	}
	fmt.Fprintf(builder, " | %s: %s", label, value.UTC().Format(time.RFC3339))
}

func (a *App) loadPipelineFinalOutputForDownload(ctx context.Context, runID, outputID string) (models.PipelineRunFinalOutput, error) {
	var output models.PipelineRunFinalOutput
	var generationStartedAt sql.NullTime
	err := a.db.QueryRow(ctx, `
		SELECT id::text, name, type, status, content, error, llm_profile,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, generation_started_at, updated_at
		FROM pipeline_run_outputs
		WHERE run_id = $1 AND id::text = $2
	`, runID, outputID).Scan(
		&output.ID,
		&output.Name,
		&output.Type,
		&output.Status,
		&output.Content,
		&output.Error,
		&output.LLMProfile,
		&output.GenerationAttempts,
		&output.ContractViolations,
		&output.RenderAttempts,
		&output.RenderFailures,
		&output.CreatedAt,
		&generationStartedAt,
		&output.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return output, sql.ErrNoRows
		}
		return output, err
	}
	output.GenerationStartedAt = nullTimePtr(generationStartedAt)
	output.GenerationDuration, output.GenerationSeconds = runquery.FinalOutputGenerationTiming(output.GenerationStartedAt, output.UpdatedAt)
	return output, nil
}

func (a *App) recordPipelineFinalOutputRenderResult(ctx context.Context, outputID string, success bool) {
	if a == nil || a.db == nil || strings.TrimSpace(outputID) == "" {
		return
	}
	failureIncrement := 0
	if !success {
		failureIncrement = 1
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_run_outputs
		SET render_attempts = render_attempts + 1,
		    render_failures = render_failures + $2
		WHERE id::text = $1
	`, outputID, failureIncrement); err != nil {
		log.Warn().Err(err).Str("output_id", outputID).Msg("Failed to record final output render audit")
	}
}

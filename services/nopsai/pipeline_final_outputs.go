package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
)

var (
	pipelineFinalOutputImageRefPattern      = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9._/-]*:[A-Za-z0-9][A-Za-z0-9._-]*\b`)
	pipelineFinalOutputDurationTokenPattern = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*(?:ms|s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\b`)
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
	Lines  []string
	Images []pipelineFinalOutputImageEvidence
}

var errPipelineFinalOutputNotCancellable = errors.New("final output is already complete")

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

func (a *App) loadPipelineFinalOutputsForGeneration(ctx context.Context, runID string) ([]pipelineFinalOutputRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, item_index, name, type, prompt, llm_profile, status, content, error,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, updated_at, dashboard_target::text
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
			&output.UpdatedAt,
			&dashboardRaw,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(dashboardRaw), &output.Dashboard)
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
	result, err := generateValidatedPipelineFinalOutput(generationCtx, client, output.Type, prompt)
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
		content, err := a.groundPipelineFinalDashboardOutputContent(ctx, runID, result.Content, output, runContext.LogEvidence)
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
		SET status = 'generating', error = '', updated_at = NOW()
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
		       created_at, updated_at, dashboard_target::text
	`, runID, outputID))
}

func (a *App) loadPipelineFinalOutputRecord(ctx context.Context, runID, outputID string) (pipelineFinalOutputRecord, error) {
	return scanPipelineFinalOutputRecord(a.db.QueryRow(ctx, `
		SELECT id::text, item_index, name, type, prompt, llm_profile, status, content, error,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, updated_at, dashboard_target::text
		FROM pipeline_run_outputs
		WHERE run_id::text = $1 AND id::text = $2
	`, runID, outputID))
}

func scanPipelineFinalOutputRecord(scanner interface{ Scan(dest ...any) error }) (pipelineFinalOutputRecord, error) {
	var output pipelineFinalOutputRecord
	var dashboardRaw string
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
		&output.UpdatedAt,
		&dashboardRaw,
	); err != nil {
		return output, err
	}
	_ = json.Unmarshal([]byte(dashboardRaw), &output.Dashboard)
	return output, nil
}

func (a *App) markDashboardRefreshOutputCancelledIfDashboard(ctx context.Context, runID string, output pipelineFinalOutputRecord) {
	if normalizePipelineFinalOutputType(output.Type) != "dashboard" {
		return
	}
	if err := a.markDashboardRefreshOutputCancelled(ctx, runID, output); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("output_id", output.ID).Msg("Failed to mark dashboard refresh output cancelled")
	}
}

func (a *App) groundPipelineFinalDashboardOutputContent(ctx context.Context, runID, content string, output pipelineFinalOutputRecord, evidence pipelineFinalOutputLogEvidence) (string, error) {
	return groundPipelineFinalDashboardOutputContent(content, output, evidence)
}

func groundPipelineFinalDashboardOutputContent(content string, output pipelineFinalOutputRecord, evidence pipelineFinalOutputLogEvidence) (string, error) {
	return content, nil
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
			builder.WriteString(fmt.Sprintf("- Step: %s | status: %s", stepName, step.Status))
			if strings.TrimSpace(step.Duration) != "" {
				builder.WriteString(" | duration: " + step.Duration)
			}
			builder.WriteString("\n")
			for _, task := range step.Tasks {
				builder.WriteString(fmt.Sprintf("  - Task: %s | status: %s", task.TaskName, task.Status))
				if task.ExitCode != nil {
					builder.WriteString(fmt.Sprintf(" | exit_code: %d", *task.ExitCode))
				}
				if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
					builder.WriteString(" | duration: " + task.FinishedAt.Sub(task.StartedAt).Round(time.Second).String())
				}
				builder.WriteString("\n")
			}
		}
	} else {
		for stepName, tasks := range tasksByStep {
			builder.WriteString("- Step: " + stepName + "\n")
			for _, task := range tasks {
				builder.WriteString(fmt.Sprintf("  - Task: %s | status: %s\n", task.TaskName, task.Status))
			}
		}
	}

	if len(childRuns) > 0 {
		builder.WriteString("\nChild runs\n")
		for _, child := range childRuns {
			builder.WriteString(fmt.Sprintf("- %s | pipeline: %s | step: %s | status: %s\n", child.RunID, child.PipelineName, child.ParentStepName, child.Status))
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
		builder.WriteString("- " + runID)
		pipeline := strings.Trim(strings.TrimSpace(item.PipelinePath+"/"+item.PipelineName), "/")
		if pipeline != "" {
			builder.WriteString(" | pipeline: " + pipeline)
		}
		if version := strings.TrimSpace(item.PipelineVersion); version != "" {
			builder.WriteString(" | version: " + version)
		}
		if status := strings.TrimSpace(item.Status); status != "" {
			builder.WriteString(" | status: " + status)
		}
		if duration := strings.TrimSpace(item.Duration); duration != "" {
			builder.WriteString(" | duration: " + duration)
		}
		writeFinalOutputTimeFragment(builder, "started_at", item.StartedAt)
		writeFinalOutputTimeFragment(builder, "finished_at", item.FinishedAt)
		if reason := strings.TrimSpace(item.FailureReason); reason != "" {
			builder.WriteString(" | failure_reason: " + reason)
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
			builder.WriteString("- " + line + "\n")
		}
	}
	if len(evidence.Lines) > 0 {
		builder.WriteString("Emitted step output lines\n")
		for _, line := range evidence.Lines {
			builder.WriteString("- " + line + "\n")
		}
	}
	builder.WriteString("Raw operational log excerpt (use only for run status and operational metadata unless it contains emitted step output)\n")
	for _, line := range logs {
		builder.WriteString("- " + line + "\n")
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
		Lines:  lines,
		Images: pipelineFinalOutputImageEvidenceFromLines(lines),
	}
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
	value, ok := pipelineFinalOutputQuotedAssignmentValue(tail)
	if !ok {
		value, ok = pipelineFinalOutputEscapedQuotedAssignmentValue(tail)
	}
	if !ok {
		return nil
	}
	return splitPipelineFinalOutputEvidenceLines(value)
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
	builder.WriteString("Use the full run context below, but do not expose secrets, credentials, tokens, or raw environment values.\n")
	builder.WriteString("Treat emitted current-run step output, including structured JSON, NDJSON, and plain-language log lines, as the primary source for business facts. If emitted step output contains values that answer the user instruction, copy those values exactly.\n")
	builder.WriteString("Do not substitute configured container images, runner/runtime images, LLM/agent metadata, operational log image-pull lines, or recent-history values for business entities such as images, versions, artifacts, services, or subjects unless the emitted step output explicitly identifies them as the requested entities.\n")
	builder.WriteString("If a file is mentioned but its contents are not present in the run context, do not infer or invent its values. Prefer operationally relevant subjects over incidental personal/noise lines unless the user specifically asks for those details.\n")
	builder.WriteString("For intent-level dashboard requests, infer the dashboard structure from the requested facts and available evidence. Use run history, step, and task duration metadata for operational run timing only when current log evidence does not answer the requested business timing.\n")
	builder.WriteString("The system output contract defines the required response envelope.\n\n")
	builder.WriteString("Output name: " + output.Name + "\n")
	builder.WriteString("Output type: " + output.Type + "\n")
	if normalizePipelineFinalOutputType(output.Type) == "dashboard" {
		writeFinalOutputLine(&builder, "Dashboard ref", output.Dashboard.Ref)
		writeFinalOutputLine(&builder, "Dashboard section", output.Dashboard.Section)
		writeFinalOutputLine(&builder, "Dashboard entry key", output.Dashboard.EntryKey)
		writeFinalOutputLine(&builder, "Dashboard publish mode", output.Dashboard.Mode)
		writeFinalOutputLine(&builder, "Dashboard preset", output.Dashboard.Preset)
	}
	builder.WriteString("Format requirements: " + pipelineFinalOutputFormatGuidance(output) + "\n\n")
	builder.WriteString("User instruction:\n")
	builder.WriteString(strings.TrimSpace(output.Prompt) + "\n\n")
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
		return dashboardFinalOutputFormatGuidance(output.Dashboard.Preset)
	default:
		return "Inside <final_output>, provide concise business-readable text."
	}
}

func dashboardFinalOutputFormatGuidance(preset string) string {
	base := `Inside <final_output>, provide only a valid DashboardSpec JSON object. Translate the user's dashboard intent into a useful dashboard from the run context; do not require the user to know schema details. Choose the dashboard structure dynamically from the prompt, pipeline definition, run metadata, recent pipeline history, step/task durations, child runs, and log evidence. If the user did not name a visualization, choose by data shape: text or callout for narrative conclusions, status/progress/properties for current state and scalar facts, table for repeated records, bar chart for categorical counts/durations/rankings, line or area chart for time series, and pie or donut chart only for bounded part-to-whole data. Use only evidence present in the run context; if requested data is absent, say it is not present rather than guessing. Available DashboardSpec blocks are status, text, callout, list, properties, table, progress, link, chart, and series. Include a non-empty title. Use one flat top-level blocks array; do not wrap dashboard output in sections or widgets, and do not put nested blocks or widgets inside a block. Use text for text and callout block bodies. Use label for display labels; key is only for table columns and chart series identifiers. Tables need columns with key/label and scalar row values. Charts need type, series, and points with label or timestamp plus finite numeric value. Do not include Markdown, HTML, CSS, JavaScript, commentary, or unsafe links. The response is validated before publication and will be retried if it does not match the DashboardSpec contract.`
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "report":
		return base + " Prefer a concise report-style presentation with summary first and supporting details after it."
	case "table":
		return base + " Prefer a scannable row-and-column presentation when the evidence naturally has repeated records."
	case "status":
		return base + " Prefer a health-and-readiness presentation that makes current state and attention items obvious."
	case "timeline":
		return base + " Prefer chronological organization when the evidence describes ordered events."
	case "comparison":
		return base + " Prefer side-by-side comparison when the evidence compares environments, versions, or options."
	case "metrics":
		return base + " Prefer a numeric metric-focused presentation with clear values, units, and trends when available."
	case "mixed", "auto":
		return base + " Choose the smallest useful presentation for the run evidence."
	default:
		return base
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
	builder.WriteString(label + ": " + value + "\n")
}

func writeFinalOutputTime(builder *strings.Builder, label string, value time.Time) {
	if value.IsZero() {
		return
	}
	builder.WriteString(label + ": " + value.UTC().Format(time.RFC3339) + "\n")
}

func writeFinalOutputTimeFragment(builder *strings.Builder, label string, value time.Time) {
	if value.IsZero() {
		return
	}
	builder.WriteString(" | " + label + ": " + value.UTC().Format(time.RFC3339))
}

func (a *App) loadPipelineFinalOutputForDownload(ctx context.Context, runID, outputID string) (models.PipelineRunFinalOutput, error) {
	var output models.PipelineRunFinalOutput
	err := a.db.QueryRow(ctx, `
		SELECT id::text, name, type, status, content, error, llm_profile,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, updated_at
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
		&output.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return output, sql.ErrNoRows
		}
		return output, err
	}
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

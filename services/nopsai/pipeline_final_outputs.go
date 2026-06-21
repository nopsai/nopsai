package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	pipelineFinalOutputFeature = "pipeline_final_output"
	finalOutputStatusPending   = "pending"
	finalOutputStatusRunning   = "generating"
	finalOutputStatusSuccess   = "success"
	finalOutputStatusFailure   = "failure"
)

type pipelineFinalOutputRecord struct {
	models.PipelineRunFinalOutput
	ItemIndex int
	Prompt    string
}

type pipelineFinalOutputRunContext struct {
	Text  string
	Scope string
}

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
				run_id, item_index, name, type, prompt, llm_profile, status, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', NOW())
			ON CONFLICT (run_id, item_index) DO UPDATE SET
				name = EXCLUDED.name,
				type = EXCLUDED.type,
				prompt = EXCLUDED.prompt,
				llm_profile = EXCLUDED.llm_profile,
				status = CASE
					WHEN pipeline_run_outputs.status = 'success' THEN pipeline_run_outputs.status
					ELSE 'pending'
				END,
				error = CASE
					WHEN pipeline_run_outputs.status = 'success' THEN pipeline_run_outputs.error
					ELSE ''
				END,
				generation_attempts = CASE
					WHEN pipeline_run_outputs.status = 'success' THEN pipeline_run_outputs.generation_attempts
					ELSE 0
				END,
				contract_violations = CASE
					WHEN pipeline_run_outputs.status = 'success' THEN pipeline_run_outputs.contract_violations
					ELSE 0
				END,
				updated_at = NOW()
		`, runID, idx, name, outputType, prompt, profileName)
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
		       created_at, updated_at
		FROM pipeline_run_outputs
		WHERE run_id = $1 AND status <> 'success'
		ORDER BY item_index ASC, created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outputs := []pipelineFinalOutputRecord{}
	for rows.Next() {
		var output pipelineFinalOutputRecord
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
		); err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return outputs, rows.Err()
}

func (a *App) generatePipelineFinalOutput(ctx context.Context, runID string, runContext pipelineFinalOutputRunContext, output pipelineFinalOutputRecord) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'generating', error = '', updated_at = NOW()
		WHERE id = $1 AND status <> 'success'
	`, output.ID); err != nil {
		return err
	}

	client, err := a.pipelineFinalOutputLLMClient(ctx, output.LLMProfile, runContext.Scope)
	if err != nil {
		_ = a.markPipelineFinalOutputFailure(ctx, output.ID, err)
		return err
	}
	prompt := buildPipelineFinalOutputPrompt(runContext.Text, output)
	result, err := generateValidatedPipelineFinalOutput(ctx, client, output.Type, prompt)
	a.recordPipelineFinalOutputAttemptUsage(ctx, runID, output, result)
	if err != nil {
		_ = a.markPipelineFinalOutputFailureWithResult(ctx, output.ID, err, result)
		return err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_run_outputs
		SET status = 'success', content = $2, error = '',
		    generation_attempts = $3, contract_violations = $4, updated_at = NOW()
		WHERE id = $1
	`, output.ID, result.Content, len(result.Attempts), result.ContractViolations); err != nil {
		return err
	}
	return nil
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
		WHERE id = $1
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
	logs, err := a.loadPipelineFinalOutputLogExcerpt(ctx, runID)
	if err != nil {
		return pipelineFinalOutputRunContext{}, err
	}

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

	if len(logs) > 0 {
		builder.WriteString("\nRecent log excerpt\n")
		for _, line := range logs {
			builder.WriteString("- " + line + "\n")
		}
	}

	return pipelineFinalOutputRunContext{Text: builder.String(), Scope: scope}, nil
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

func buildPipelineFinalOutputPrompt(runContext string, output pipelineFinalOutputRecord) string {
	var builder strings.Builder
	builder.WriteString("You are creating a polished final deliverable for an enterprise pipeline run.\n")
	builder.WriteString("Use the full run context below, but do not expose secrets, credentials, tokens, or raw environment values.\n")
	builder.WriteString("The system output contract defines the required response envelope.\n\n")
	builder.WriteString("Output name: " + output.Name + "\n")
	builder.WriteString("Output type: " + output.Type + "\n")
	builder.WriteString("Format requirements: " + pipelineFinalOutputFormatGuidance(output.Type) + "\n\n")
	builder.WriteString("User instruction:\n")
	builder.WriteString(strings.TrimSpace(output.Prompt) + "\n\n")
	builder.WriteString("Run context:\n")
	builder.WriteString(runContext)
	return builder.String()
}

func pipelineFinalOutputFormatGuidance(outputType string) string {
	switch normalizePipelineFinalOutputType(outputType) {
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
	default:
		return "Inside <final_output>, provide concise business-readable text."
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

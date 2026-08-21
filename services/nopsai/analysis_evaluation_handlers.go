package nopsai

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/llmclient"
	"nopsai/pkg/models"
)

const (
	maxAnalysisEvaluationPromptBytes = 120000
	analysisEvaluationFeature        = "analysis_evaluation"
)

type analysisEvaluationRequest struct {
	SubjectType        string `json:"subject_type"`
	SubjectID          string `json:"subject_id"`
	SubjectLabel       string `json:"subject_label"`
	Scope              string `json:"scope"`
	SelectedLLMProfile string `json:"selected_llm_profile"`
	Prompt             string `json:"prompt"`
}

type analysisEvaluationResponse struct {
	Content     string                  `json:"content"`
	ProfileName string                  `json:"profile_name"`
	Provider    string                  `json:"provider,omitempty"`
	Model       string                  `json:"model,omitempty"`
	GeneratedAt time.Time               `json:"generated_at"`
	Usage       analysisEvaluationUsage `json:"usage"`
}

type analysisEvaluationUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	Estimated        bool  `json:"estimated"`
	DurationMS       int64 `json:"duration_ms"`
}

func (a *App) registerAnalysisRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/analysis/evaluate", a.handleEvaluateAnalysis)
}

func (a *App) handleEvaluateAnalysis(w http.ResponseWriter, r *http.Request) {
	if !a.requireAnalysisEvaluationEnabled(w) {
		return
	}
	if _, ok := a.requireAssistantUserID(w, r); !ok {
		return
	}

	var req analysisEvaluationRequest
	if err := httpapi.DecodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid analysis evaluation payload", http.StatusBadRequest)
		return
	}
	req = normalizeAnalysisEvaluationRequest(req)
	if err := validateAnalysisEvaluationRequest(req, a.assistantConfig()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conversation := assistantConversation{
		SelectedLLMProfile: req.SelectedLLMProfile,
		Scope:              req.Scope,
		Memory: assistantConversationMemory{
			SelectedScope:    req.Scope,
			SelectedRun:      analysisSelectedRun(req),
			SelectedPipeline: analysisSelectedPipeline(req),
		},
	}
	profileName, profile, client, ok, reason := a.assistantLLMClientForTurn(r.Context(), conversation, req.SelectedLLMProfile)
	if !ok {
		if strings.TrimSpace(reason) == "" {
			reason = "no usable LLM profile is available"
		}
		http.Error(w, reason, http.StatusBadRequest)
		return
	}

	start := time.Now()
	completion, err := client.Complete(r.Context(), req.Prompt)
	if err != nil {
		http.Error(w, "analysis evaluation failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	content := strings.TrimSpace(completion.Text)
	if content == "" {
		http.Error(w, "analysis evaluation returned an empty response", http.StatusBadGateway)
		return
	}

	a.recordAnalysisEvaluationUsage(r, req, completion.Usage)

	_ = httpapi.WriteJSON(w, http.StatusOK, analysisEvaluationResponse{
		Content:     content,
		ProfileName: profileName,
		Provider:    profile.Provider,
		Model:       profile.Model,
		GeneratedAt: time.Now().UTC(),
		Usage:       analysisEvaluationUsageFromLLMUsage(completion.Usage, time.Since(start)),
	})
}

// recordAnalysisEvaluationUsage files the spend of an analysis evaluation.
//
// An evaluation of a run is attributed to that run so it lands in the run's
// spend. An evaluation of a pipeline has no run to attribute to and is recorded
// standalone; either way the money is accounted for, because a call that costs
// real money and appears in no total is how a spend figure quietly drifts away
// from the invoice.
func (a *App) recordAnalysisEvaluationUsage(r *http.Request, req analysisEvaluationRequest, usage llmclient.Usage) {
	report := models.AIUsageReport{
		Feature:           analysisEvaluationFeature,
		Provider:          usage.Provider,
		ProviderModel:     usage.Model,
		LLMProfile:        usage.Profile,
		PromptTokens:      usage.PromptTokens,
		CompletionTokens:  usage.CompletionTokens,
		TotalTokens:       usage.TotalTokens,
		CachedInputTokens: usage.CachedInputTokens,
		CacheWriteTokens:  usage.CacheWriteTokens,
		Estimated:         usage.Estimated,
		Metadata: map[string]any{
			"subject_type": req.SubjectType,
			"subject_id":   req.SubjectID,
		},
	}
	if report.PromptTokens == 0 && report.CompletionTokens == 0 && report.TotalTokens == 0 {
		return
	}

	ctx := r.Context()
	var err error
	if runID := analysisSelectedRun(req); runID != "" {
		err = a.recordAIUsage(ctx, runID, report)
	} else {
		subject, _ := a.currentAAASubject(r)
		err = a.recordStandaloneAIUsage(ctx, subject, report)
	}
	if err != nil {
		log.Error().Err(err).
			Str("subject_type", req.SubjectType).
			Str("subject_id", req.SubjectID).
			Msg("Failed to record analysis evaluation AI usage")
	}
}

func (a *App) requireAnalysisEvaluationEnabled(w http.ResponseWriter) bool {
	if a == nil || a.cfg == nil {
		http.Error(w, "analysis evaluation is unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !a.assistantConfig().Enabled {
		http.Error(w, "assistant is disabled", http.StatusNotFound)
		return false
	}
	return true
}

func normalizeAnalysisEvaluationRequest(req analysisEvaluationRequest) analysisEvaluationRequest {
	req.SubjectType = strings.ToLower(strings.TrimSpace(req.SubjectType))
	req.SubjectID = strings.TrimSpace(req.SubjectID)
	req.SubjectLabel = strings.TrimSpace(req.SubjectLabel)
	req.Scope = strings.Trim(strings.TrimSpace(req.Scope), "/")
	req.SelectedLLMProfile = strings.TrimSpace(req.SelectedLLMProfile)
	req.Prompt = strings.TrimSpace(req.Prompt)
	return req
}

func validateAnalysisEvaluationRequest(req analysisEvaluationRequest, cfg config.AssistantConfig) error {
	if req.Prompt == "" {
		return errors.New("prompt is required")
	}
	switch req.SubjectType {
	case "run", "pipeline", "team", "resource":
	default:
		return errors.New("subject_type must be one of run, pipeline, team, or resource")
	}
	if req.SubjectID == "" {
		return errors.New("subject_id is required")
	}
	if len([]byte(req.Prompt)) > maxAnalysisEvaluationPromptBytes {
		return errors.New("prompt exceeds analysis evaluation input limit")
	}
	return validateAnalysisEvaluationFeature(req.SubjectType, cfg)
}

func validateAnalysisEvaluationFeature(subjectType string, cfg config.AssistantConfig) error {
	if subjectType == "run" && !config.AssistantFeatureFlagEnabled(cfg.Features.PipelineDebugging) {
		return errors.New("assistant run debugging is disabled")
	}
	if subjectType != "run" && !config.AssistantFeatureFlagEnabled(cfg.Features.MaintenanceRecommendations) {
		return errors.New("assistant maintenance recommendations are disabled")
	}
	return nil
}

func analysisSelectedRun(req analysisEvaluationRequest) string {
	if req.SubjectType == "run" {
		return req.SubjectID
	}
	return ""
}

func analysisSelectedPipeline(req analysisEvaluationRequest) string {
	if req.SubjectType == "pipeline" {
		return req.SubjectID
	}
	return ""
}

func analysisEvaluationUsageFromLLMUsage(usage llmclient.Usage, duration time.Duration) analysisEvaluationUsage {
	total := nonNegativeInt64(usage.TotalTokens)
	prompt := nonNegativeInt64(usage.PromptTokens)
	completion := nonNegativeInt64(usage.CompletionTokens)
	if total == 0 {
		total = prompt + completion
	}
	return analysisEvaluationUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
		Estimated:        usage.Estimated,
		DurationMS:       assistantDurationMilliseconds(duration),
	}
}
